package elasticsearch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// fakeES serves a canned _cluster/health and _cat/allocation.
func fakeES(t *testing.T, health, alloc string) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/_cluster/health", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(health))
	})
	mux.HandleFunc("/_cat/allocation", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(alloc))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

func run(t *testing.T, cfg engine.ElasticsearchConfig, target engine.ElasticsearchTarget) []engine.Finding {
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

const allocOK = `[{"node":"es-01","disk.percent":"40"},{"node":"es-02","disk.percent":"42"}]`

func TestGreenClusterOK(t *testing.T) {
	url := fakeES(t,
		`{"cluster_name":"c","status":"green","number_of_nodes":3,"unassigned_shards":0,"active_shards_percent_as_number":100}`,
		allocOK)
	fs := run(t, engine.ElasticsearchConfig{DiskWarnPct: 85, DiskCritPct: 90},
		engine.ElasticsearchTarget{Name: "es", URL: url})
	health, _ := findingFor(fs, "es")
	if health.Status != engine.OK {
		t.Fatalf("green should be OK, got %s: %s", health.Status, health.Message)
	}
}

func TestYellowIsWarn(t *testing.T) {
	url := fakeES(t,
		`{"status":"yellow","number_of_nodes":2,"unassigned_shards":5,"active_shards_percent_as_number":80}`,
		allocOK)
	fs := run(t, engine.ElasticsearchConfig{DiskWarnPct: 85, DiskCritPct: 90},
		engine.ElasticsearchTarget{Name: "es", URL: url})
	health, _ := findingFor(fs, "es")
	if health.Status != engine.WARN || !strings.Contains(health.Message, "unassigned") {
		t.Fatalf("yellow should WARN with unassigned info, got %s: %s", health.Status, health.Message)
	}
}

func TestRedIsBad(t *testing.T) {
	url := fakeES(t,
		`{"status":"red","number_of_nodes":1,"unassigned_shards":20,"active_shards_percent_as_number":50}`,
		allocOK)
	fs := run(t, engine.ElasticsearchConfig{DiskWarnPct: 85, DiskCritPct: 90},
		engine.ElasticsearchTarget{Name: "es", URL: url})
	health, _ := findingFor(fs, "es")
	if health.Status != engine.BAD {
		t.Fatalf("red should be BAD, got %s: %s", health.Status, health.Message)
	}
}

func TestExpectNodesShortfall(t *testing.T) {
	url := fakeES(t,
		`{"status":"green","number_of_nodes":2,"unassigned_shards":0,"active_shards_percent_as_number":100}`,
		allocOK)
	fs := run(t, engine.ElasticsearchConfig{DiskWarnPct: 85, DiskCritPct: 90},
		engine.ElasticsearchTarget{Name: "es", URL: url, ExpectNodes: 3})
	nodes, ok := findingFor(fs, "es/nodes")
	if !ok || nodes.Status != engine.BAD {
		t.Fatalf("want BAD nodes finding, got %+v", nodes)
	}
}

func TestDiskWatermark(t *testing.T) {
	alloc := `[{"node":"es-01","disk.percent":"40"},{"node":"es-02","disk.percent":"88"},{"node":"es-03","disk.percent":"95"},{"node":"UNASSIGNED","disk.percent":""}]`
	url := fakeES(t,
		`{"status":"green","number_of_nodes":3,"unassigned_shards":0,"active_shards_percent_as_number":100}`,
		alloc)
	fs := run(t, engine.ElasticsearchConfig{DiskWarnPct: 85, DiskCritPct: 90},
		engine.ElasticsearchTarget{Name: "es", URL: url})

	if f, _ := findingFor(fs, "es/disk/es-01"); f.Status != engine.OK {
		t.Errorf("es-01 40%% should be OK, got %s", f.Status)
	}
	if f, _ := findingFor(fs, "es/disk/es-02"); f.Status != engine.WARN {
		t.Errorf("es-02 88%% should be WARN, got %s", f.Status)
	}
	if f, _ := findingFor(fs, "es/disk/es-03"); f.Status != engine.BAD {
		t.Errorf("es-03 95%% should be BAD, got %s", f.Status)
	}
	if _, ok := findingFor(fs, "es/disk/UNASSIGNED"); ok {
		t.Error("UNASSIGNED bucket (empty disk) should be skipped")
	}
}

func TestUnreachableIsError(t *testing.T) {
	fs := run(t, engine.ElasticsearchConfig{DiskWarnPct: 85, DiskCritPct: 90},
		engine.ElasticsearchTarget{Name: "es", URL: "http://127.0.0.1:1"})
	health, _ := findingFor(fs, "es")
	if health.Status != engine.ERROR {
		t.Fatalf("unreachable should be ERROR, got %s: %s", health.Status, health.Message)
	}
}

func TestBasicAuthSent(t *testing.T) {
	var gotUser, gotPass string
	var gotOK bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, gotPass, gotOK = r.BasicAuth()
		if r.URL.Path == "/_cluster/health" {
			_, _ = w.Write([]byte(`{"status":"green","number_of_nodes":1,"active_shards_percent_as_number":100}`))
			return
		}
		_, _ = w.Write([]byte(allocOK))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("ES_PW", "s3cret")
	run(t, engine.ElasticsearchConfig{DiskWarnPct: 85, DiskCritPct: 90},
		engine.ElasticsearchTarget{URL: srv.URL, Username: "elastic", PasswordEnv: "ES_PW"})
	if !gotOK || gotUser != "elastic" || gotPass != "s3cret" {
		t.Fatalf("basic auth not sent correctly: ok=%v user=%q pass=%q", gotOK, gotUser, gotPass)
	}
}
