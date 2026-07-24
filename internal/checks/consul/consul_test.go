package consul

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// fakeConsul serves canned responses for the API paths we use.
type fakeConsul struct {
	leader   string   // JSON value for /v1/status/leader (already a Go string)
	peers    []string // /v1/status/peers
	critical string   // JSON body for /v1/health/state/critical
	warning  string   // JSON body for /v1/health/state/warning
	kv       map[string]bool
	token    string // if set, requests must carry this X-Consul-Token
}

func serve(t *testing.T, f fakeConsul) string {
	t.Helper()
	enc := func(w http.ResponseWriter, v string) { _, _ = w.Write([]byte(v)) }
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f.token != "" && r.Header.Get("X-Consul-Token") != f.token {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		switch {
		case r.URL.Path == "/v1/status/leader":
			enc(w, `"`+f.leader+`"`)
		case r.URL.Path == "/v1/status/peers":
			b := "["
			for i, p := range f.peers {
				if i > 0 {
					b += ","
				}
				b += `"` + p + `"`
			}
			enc(w, b+"]")
		case r.URL.Path == "/v1/health/state/critical":
			enc(w, orEmpty(f.critical))
		case r.URL.Path == "/v1/health/state/warning":
			enc(w, orEmpty(f.warning))
		case strings.HasPrefix(r.URL.Path, "/v1/kv/"):
			key := strings.TrimPrefix(r.URL.Path, "/v1/kv/")
			if f.kv[key] {
				enc(w, `[{"Key":"`+key+`"}]`)
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://")
}

func orEmpty(s string) string {
	if s == "" {
		return "[]"
	}
	return s
}

func run(t *testing.T, cfg engine.ConsulConfig) []engine.Finding {
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

func TestHealthyCluster(t *testing.T) {
	target := serve(t, fakeConsul{leader: "10.0.0.1:8300", peers: []string{"10.0.0.1:8300", "10.0.0.2:8300", "10.0.0.3:8300"}})
	f := byTarget(run(t, engine.ConsulConfig{Targets: []string{target}, ExpectPeers: 3}))
	if got := f["raft-leader"]; got.Status != engine.OK {
		t.Errorf("leader: atteso OK, avuto %s (%s)", got.Status, got.Message)
	}
	if got := f["raft-peers"]; got.Status != engine.OK || !strings.Contains(got.Message, "3 peer") {
		t.Errorf("peers: atteso OK 3, avuto %s (%s)", got.Status, got.Message)
	}
}

func TestNoLeaderIsBad(t *testing.T) {
	target := serve(t, fakeConsul{leader: "", peers: []string{"a", "b", "c"}})
	if got := byTarget(run(t, engine.ConsulConfig{Targets: []string{target}}))["raft-leader"]; got.Status != engine.BAD {
		t.Errorf("no leader: atteso BAD, avuto %s (%s)", got.Status, got.Message)
	}
}

func TestQuorumLostIsBad(t *testing.T) {
	target := serve(t, fakeConsul{leader: "x", peers: []string{"a"}}) // 1 di 3 attesi → quorum perso
	if got := byTarget(run(t, engine.ConsulConfig{Targets: []string{target}, ExpectPeers: 3}))["raft-peers"]; got.Status != engine.BAD {
		t.Errorf("quorum perso: atteso BAD, avuto %s (%s)", got.Status, got.Message)
	}
}

func TestMissingPeerIsWarn(t *testing.T) {
	target := serve(t, fakeConsul{leader: "x", peers: []string{"a", "b"}}) // 2 di 3 → quorum ok ma manca uno
	if got := byTarget(run(t, engine.ConsulConfig{Targets: []string{target}, ExpectPeers: 3}))["raft-peers"]; got.Status != engine.WARN {
		t.Errorf("peer mancante: atteso WARN, avuto %s (%s)", got.Status, got.Message)
	}
}

func TestCriticalAndWarningChecks(t *testing.T) {
	target := serve(t, fakeConsul{
		leader:   "x",
		peers:    []string{"a", "b", "c"},
		critical: `[{"Node":"n1","CheckID":"service:web","Name":"web health","Status":"critical","ServiceName":"web"}]`,
		warning:  `[{"Node":"n2","CheckID":"disk","Name":"disk space","Status":"warning"}]`,
	})
	f := byTarget(run(t, engine.ConsulConfig{Targets: []string{target}}))
	if got := f["web@n1"]; got.Status != engine.BAD {
		t.Errorf("check critical: atteso BAD, avuto %s (%s)", got.Status, got.Message)
	}
	if got := f["disk@n2"]; got.Status != engine.WARN {
		t.Errorf("check warning: atteso WARN, avuto %s (%s)", got.Status, got.Message)
	}
}

func TestKVKeysAndToken(t *testing.T) {
	t.Setenv("CONSUL_TOKEN", "secret-token")
	target := serve(t, fakeConsul{
		leader: "x", peers: []string{"a", "b", "c"},
		kv:    map[string]bool{"config/present": true},
		token: "secret-token",
	})
	f := byTarget(run(t, engine.ConsulConfig{
		Targets: []string{target}, TokenEnv: "CONSUL_TOKEN",
		KVKeys: []string{"config/present", "config/missing"},
	}))
	if got := f["kv/config/present"]; got.Status != engine.OK {
		t.Errorf("kv presente: atteso OK, avuto %s (%s)", got.Status, got.Message)
	}
	if got := f["kv/config/missing"]; got.Status != engine.BAD {
		t.Errorf("kv mancante: atteso BAD, avuto %s (%s)", got.Status, got.Message)
	}
}

func TestUnreachableIsError(t *testing.T) {
	f := run(t, engine.ConsulConfig{Targets: []string{"127.0.0.1:1"}})
	if len(f) == 0 || f[0].Status != engine.ERROR {
		t.Errorf("irraggiungibile: atteso ERROR, avuto %v", f)
	}
}
