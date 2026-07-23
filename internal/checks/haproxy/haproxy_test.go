package haproxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// statsHeader is a trimmed HAProxy CSV header with the columns we consume.
const statsHeader = "# pxname,svname,scur,slim,status,\n"

// startStats serves the given CSV body at /stats;csv.
func startStats(t *testing.T, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RequestURI() != "/stats;csv" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://")
}

func run(t *testing.T, cfg engine.HAProxyConfig) []engine.Finding {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return New(cfg).Run(ctx)
}

func byTarget(findings []engine.Finding) map[string]engine.Finding {
	m := map[string]engine.Finding{}
	for _, f := range findings {
		m[f.Target] = f
	}
	return m
}

func TestServerAndBackendStatuses(t *testing.T) {
	csv := statsHeader +
		"web,web1,3,100,UP,\n" +
		"web,web2,0,100,DOWN,\n" +
		"web,web3,0,100,MAINT,\n" +
		"web,BACKEND,3,200,UP,\n" +
		"api,api1,0,50,DOWN,\n" +
		"api,BACKEND,0,50,DOWN,\n" +
		"stats,FRONTEND,1,0,OPEN,\n"
	f := byTarget(run(t, engine.HAProxyConfig{Targets: []string{startStats(t, csv)}, Path: "/stats;csv"}))

	cases := map[string]engine.Status{
		"web/web1":    engine.OK,
		"web/web2":    engine.BAD,
		"web/web3":    engine.WARN,
		"web/BACKEND": engine.OK,
		"api/api1":    engine.BAD,
		"api/BACKEND": engine.BAD,
	}
	for label, want := range cases {
		if got := f[label].Status; got != want {
			t.Errorf("%s: atteso %s, avuto %s (%s)", label, want, got, f[label].Message)
		}
	}
	if _, ok := f["stats/FRONTEND"]; ok {
		t.Errorf("i FRONTEND dovrebbero essere ignorati, trovato: %v", f["stats/FRONTEND"])
	}
}

func TestBackendDownMessage(t *testing.T) {
	csv := statsHeader + "api,BACKEND,0,50,DOWN,\n"
	f := byTarget(run(t, engine.HAProxyConfig{Targets: []string{startStats(t, csv)}}))
	if got := f["api/BACKEND"]; got.Status != engine.BAD || !strings.Contains(got.Message, "nessun server") {
		t.Errorf("backend down: atteso BAD 'nessun server', avuto %s (%s)", got.Status, got.Message)
	}
}

func TestSessionUsageWarn(t *testing.T) {
	csv := statsHeader +
		"web,web1,90,100,UP,\n" + // 90% → oltre soglia 80
		"web,web2,10,100,UP,\n" // 10% → ok
	f := byTarget(run(t, engine.HAProxyConfig{
		Targets: []string{startStats(t, csv)}, SessionWarnPct: 80,
	}))
	if got := f["web/web1"]; got.Status != engine.WARN || !strings.Contains(got.Message, "sessioni") {
		t.Errorf("web1 90%%: atteso WARN sessioni, avuto %s (%s)", got.Status, got.Message)
	}
	if got := f["web/web2"]; got.Status != engine.OK {
		t.Errorf("web2 10%%: atteso OK, avuto %s (%s)", got.Status, got.Message)
	}
}

func TestBasicAuth(t *testing.T) {
	t.Setenv("HAPROXY_PASS", "s3cr3t")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "admin" || pass != "s3cr3t" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(statsHeader + "web,web1,1,100,UP,\n"))
	}))
	t.Cleanup(srv.Close)
	target := strings.TrimPrefix(srv.URL, "http://")

	// Senza credenziali → 401 → ERROR.
	if f := run(t, engine.HAProxyConfig{Targets: []string{target}}); f[0].Status != engine.ERROR {
		t.Errorf("senza auth: atteso ERROR, avuto %s (%s)", f[0].Status, f[0].Message)
	}
	// Con credenziali (password da env) → OK.
	f := byTarget(run(t, engine.HAProxyConfig{
		Targets: []string{target}, AuthUser: "admin", AuthPassEnv: "HAPROXY_PASS",
	}))
	if got := f["web/web1"]; got.Status != engine.OK {
		t.Errorf("con auth: atteso OK, avuto %s (%s)", got.Status, got.Message)
	}
}

func TestUnreachableIsError(t *testing.T) {
	f := run(t, engine.HAProxyConfig{Targets: []string{"127.0.0.1:1"}})
	if len(f) == 0 || f[0].Status != engine.ERROR {
		t.Errorf("irraggiungibile: atteso ERROR, avuto %v", f)
	}
}

func TestTargetsDefaultPort(t *testing.T) {
	check := New(engine.HAProxyConfig{Port: 8404, Targets: []string{"lb1", "lb2:1940"}})
	targets, err := check.Targets()
	if err != nil {
		t.Fatal(err)
	}
	if targets[0] != "lb1:8404" || targets[1] != "lb2:1940" {
		t.Errorf("targets/porta di default sbagliati: %v", targets)
	}
}
