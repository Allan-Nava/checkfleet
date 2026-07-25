package clickhouse

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// fakeCH serves /ping and /?query=... . replicasTSV is returned for the
// system.replicas query; empty means "no replicated tables".
func fakeCH(t *testing.T, ping bool, version, replicasTSV string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ping" {
			if !ping {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_, _ = w.Write([]byte("Ok.\n"))
			return
		}
		q := r.URL.Query().Get("query")
		switch {
		case strings.Contains(q, "version()"):
			_, _ = w.Write([]byte(version + "\n"))
		case strings.Contains(q, "system.replicas"):
			_, _ = w.Write([]byte(replicasTSV))
		default:
			_, _ = w.Write([]byte("1\n"))
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func run(t *testing.T, cfg engine.ClickHouseConfig, target engine.ClickHouseTarget) []engine.Finding {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return New(cfg).probe(ctx, target)
}

func findingFor(fs []engine.Finding, target string) (engine.Finding, bool) {
	for _, f := range fs {
		if f.Target == target {
			return f, true
		}
	}
	return engine.Finding{}, false
}

func defaults() engine.ClickHouseConfig {
	return engine.ClickHouseConfig{DelayWarnSeconds: 30, DelayCritSeconds: 300}
}

func TestReachableOK(t *testing.T) {
	url := fakeCH(t, true, "24.3.1", "")
	fs := run(t, defaults(), engine.ClickHouseTarget{Name: "ch", URL: url})
	f, _ := findingFor(fs, "ch")
	if f.Status != engine.OK || !strings.Contains(f.Message, "24.3.1") {
		t.Fatalf("want OK with version, got %s: %s", f.Status, f.Message)
	}
}

func TestPingDownIsError(t *testing.T) {
	url := fakeCH(t, false, "24.3.1", "")
	f, _ := findingFor(run(t, defaults(), engine.ClickHouseTarget{Name: "ch", URL: url}), "ch")
	if f.Status != engine.ERROR {
		t.Fatalf("ping down should be ERROR, got %s: %s", f.Status, f.Message)
	}
}

func TestReplicaReadOnlyIsBad(t *testing.T) {
	tsv := "db\tevents\t1\t0\n"
	url := fakeCH(t, true, "24.3", tsv)
	f, ok := findingFor(run(t, defaults(), engine.ClickHouseTarget{Name: "ch", URL: url}), "ch/db.events")
	if !ok || f.Status != engine.BAD || !strings.Contains(f.Message, "read-only") {
		t.Fatalf("read-only replica should be BAD, got %+v", f)
	}
}

func TestReplicaDelayThresholds(t *testing.T) {
	// one healthy (delay 5), one warn (60), one crit (400)
	tsv := "db\tt_ok\t0\t5\ndb\tt_warn\t0\t60\ndb\tt_crit\t0\t400\n"
	url := fakeCH(t, true, "24.3", tsv)
	fs := run(t, defaults(), engine.ClickHouseTarget{Name: "ch", URL: url})
	if _, ok := findingFor(fs, "ch/db.t_ok"); ok {
		t.Error("healthy replica should produce no finding")
	}
	if f, _ := findingFor(fs, "ch/db.t_warn"); f.Status != engine.WARN {
		t.Errorf("60s delay should WARN, got %s", f.Status)
	}
	if f, _ := findingFor(fs, "ch/db.t_crit"); f.Status != engine.BAD {
		t.Errorf("400s delay should BAD, got %s", f.Status)
	}
}

func TestUnreachableIsError(t *testing.T) {
	f, _ := findingFor(run(t, defaults(), engine.ClickHouseTarget{Name: "ch", URL: "http://127.0.0.1:1"}), "ch")
	if f.Status != engine.ERROR {
		t.Fatalf("unreachable should be ERROR, got %s: %s", f.Status, f.Message)
	}
}
