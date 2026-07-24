package rabbitmq

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

func serve(t *testing.T, nodes, queues string) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/overview", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"rabbitmq_version":"3.13.0","cluster_name":"rabbit@h"}`))
	})
	mux.HandleFunc("/api/nodes", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(nodes)) })
	mux.HandleFunc("/api/queues", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(queues)) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://")
}

func run(t *testing.T, target string) map[string]engine.Finding {
	t.Helper()
	cfg := engine.RabbitMQConfig{Targets: []string{target}, Username: "guest", QueueWarnDepth: 1000, QueueCritDepth: 50000}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	m := map[string]engine.Finding{}
	for _, f := range New(cfg).Run(ctx) {
		m[f.Target] = f
	}
	return m
}

func TestHealthy(t *testing.T) {
	target := serve(t,
		`[{"name":"rabbit@n1","running":true,"mem_alarm":false,"disk_free_alarm":false}]`,
		`[{"name":"work","vhost":"/","messages":5,"consumers":2}]`)
	f := run(t, target)
	if f["node/n1"].Status != engine.OK {
		t.Errorf("node running: want OK, got %s (%s)", f["node/n1"].Status, f["node/n1"].Message)
	}
	if f["queue/work"].Status != engine.OK {
		t.Errorf("queue ok: want OK, got %s (%s)", f["queue/work"].Status, f["queue/work"].Message)
	}
}

func TestNodeAlarmsAndDown(t *testing.T) {
	target := serve(t,
		`[{"name":"rabbit@n1","running":false},{"name":"rabbit@n2","running":true,"mem_alarm":true}]`,
		`[]`)
	f := run(t, target)
	if f["node/n1"].Status != engine.BAD {
		t.Errorf("node down: want BAD, got %s", f["node/n1"].Status)
	}
	if got := f["node/n2"]; got.Status != engine.BAD || !strings.Contains(got.Message, "memory") {
		t.Errorf("mem alarm: want BAD, got %s (%s)", got.Status, got.Message)
	}
}

func TestQueueDepthAndNoConsumer(t *testing.T) {
	target := serve(t,
		`[{"name":"rabbit@n1","running":true}]`,
		`[{"name":"big","vhost":"/","messages":60000,"consumers":1},
		  {"name":"stuck","vhost":"/","messages":10,"consumers":0},
		  {"name":"warn","vhost":"/","messages":2000,"consumers":1}]`)
	f := run(t, target)
	if f["queue/big"].Status != engine.BAD {
		t.Errorf("queue 60k: want BAD, got %s (%s)", f["queue/big"].Status, f["queue/big"].Message)
	}
	if got := f["queue/stuck"]; got.Status != engine.WARN || !strings.Contains(got.Message, "no consumer") {
		t.Errorf("queue without consumer: want WARN, got %s (%s)", got.Status, got.Message)
	}
	if f["queue/warn"].Status != engine.WARN {
		t.Errorf("queue 2000: want WARN, got %s (%s)", f["queue/warn"].Status, f["queue/warn"].Message)
	}
}

func TestUnreachableIsError(t *testing.T) {
	f := run(t, "127.0.0.1:1")
	for _, v := range f {
		if v.Status != engine.ERROR {
			t.Errorf("unreachable: want ERROR, got %s", v.Status)
		}
	}
}
