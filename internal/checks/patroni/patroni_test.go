package patroni

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// serveCluster spins an httptest server returning the given /cluster JSON.
func serveCluster(t *testing.T, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cluster" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://")
}

func run(t *testing.T, cfg engine.PatroniConfig) []engine.Finding {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return New(cfg).Run(ctx)
}

func byTarget(f []engine.Finding) map[string]engine.Finding {
	m := map[string]engine.Finding{}
	for _, x := range f {
		m[x.Target] = x
	}
	return m
}

const healthy = `{"scope":"pgcluster","members":[
  {"name":"pg1","role":"leader","state":"running","timeline":5},
  {"name":"pg2","role":"replica","state":"streaming","timeline":5,"lag":0},
  {"name":"pg3","role":"replica","state":"streaming","timeline":5,"lag":1048576}
]}`

func cfgFor(target string) engine.PatroniConfig {
	return engine.PatroniConfig{Targets: []string{target}, LagWarnBytes: 32 << 20, LagCritBytes: 128 << 20}
}

func TestHealthyCluster(t *testing.T) {
	f := byTarget(run(t, cfgFor(serveCluster(t, healthy))))
	if got := f["pgcluster"]; got.Status != engine.OK || !strings.Contains(got.Message, "pg1") {
		t.Errorf("leader: want OK pg1, got %s (%s)", got.Status, got.Message)
	}
	if f["pg2"].Status != engine.OK || f["pg3"].Status != engine.OK {
		t.Errorf("repliche sane attese OK: %+v %+v", f["pg2"], f["pg3"])
	}
}

func TestNoLeaderIsBad(t *testing.T) {
	body := `{"scope":"c","members":[
	  {"name":"pg2","role":"replica","state":"running","timeline":5},
	  {"name":"pg3","role":"replica","state":"running","timeline":5}]}`
	if got := byTarget(run(t, cfgFor(serveCluster(t, body))))["c"]; got.Status != engine.BAD {
		t.Errorf("no leader: want BAD, got %s (%s)", got.Status, got.Message)
	}
}

func TestSplitBrainIsWarn(t *testing.T) {
	body := `{"scope":"c","members":[
	  {"name":"pg1","role":"leader","state":"running","timeline":5},
	  {"name":"pg2","role":"leader","state":"running","timeline":6}]}`
	if got := byTarget(run(t, cfgFor(serveCluster(t, body))))["c"]; got.Status != engine.WARN {
		t.Errorf("two leaders: want WARN, got %s (%s)", got.Status, got.Message)
	}
}

func TestLagThresholds(t *testing.T) {
	body := `{"scope":"c","members":[
	  {"name":"pg1","role":"leader","state":"running","timeline":5},
	  {"name":"warn","role":"replica","state":"streaming","timeline":5,"lag":67108864},
	  {"name":"bad","role":"replica","state":"streaming","timeline":5,"lag":268435456}]}`
	f := byTarget(run(t, cfgFor(serveCluster(t, body))))
	if f["warn"].Status != engine.WARN {
		t.Errorf("lag 64MiB: want WARN, got %s (%s)", f["warn"].Status, f["warn"].Message)
	}
	if f["bad"].Status != engine.BAD {
		t.Errorf("lag 256MiB: want BAD, got %s (%s)", f["bad"].Status, f["bad"].Message)
	}
}

func TestBadReplicaStateAndTimeline(t *testing.T) {
	body := `{"scope":"c","members":[
	  {"name":"pg1","role":"leader","state":"running","timeline":7},
	  {"name":"stopped","role":"replica","state":"stopped","timeline":7},
	  {"name":"oldtl","role":"replica","state":"streaming","timeline":5,"lag":0}]}`
	f := byTarget(run(t, cfgFor(serveCluster(t, body))))
	if f["stopped"].Status != engine.BAD {
		t.Errorf("replica stopped: want BAD, got %s (%s)", f["stopped"].Status, f["stopped"].Message)
	}
	if f["oldtl"].Status != engine.WARN || !strings.Contains(f["oldtl"].Message, "timeline") {
		t.Errorf("timeline divergence: want WARN, got %s (%s)", f["oldtl"].Status, f["oldtl"].Message)
	}
}

func TestUnknownLagIsOK(t *testing.T) {
	body := `{"scope":"c","members":[
	  {"name":"pg1","role":"leader","state":"running","timeline":5},
	  {"name":"pg2","role":"replica","state":"streaming","timeline":5,"lag":"unknown"}]}`
	if got := byTarget(run(t, cfgFor(serveCluster(t, body))))["pg2"]; got.Status != engine.OK || !strings.Contains(got.Message, "unknown") {
		t.Errorf("lag unknown: want OK with note, got %s (%s)", got.Status, got.Message)
	}
}

func TestUnreachableIsError(t *testing.T) {
	f := run(t, cfgFor("127.0.0.1:1"))
	if len(f) == 0 || f[0].Status != engine.ERROR {
		t.Errorf("unreachable: want ERROR, got %v", f)
	}
}

func TestTargetsDefaultPort(t *testing.T) {
	targets, err := New(engine.PatroniConfig{Port: 8008, Targets: []string{"pg1", "pg2:8009"}}).Targets()
	if err != nil {
		t.Fatal(err)
	}
	if targets[0] != "pg1:8008" || targets[1] != "pg2:8009" {
		t.Errorf("porta di default sbagliata: %v", targets)
	}
}
