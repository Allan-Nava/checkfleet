package etcd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

type fake struct {
	health  string // /health "health" field
	leader  string // maintenance/status leader
	members int    // member/list size
	token   string // if set, require this Authorization header on v3 calls
}

func fakeEtcd(t *testing.T, f fake) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"health":"` + f.health + `"}`))
	})
	guard := func(w http.ResponseWriter, r *http.Request) bool {
		if f.token != "" && r.Header.Get("Authorization") != f.token {
			w.WriteHeader(http.StatusUnauthorized)
			return false
		}
		return true
	}
	mux.HandleFunc("/v3/maintenance/status", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		_, _ = w.Write([]byte(`{"version":"3.5.9","leader":"` + f.leader + `"}`))
	})
	mux.HandleFunc("/v3/cluster/member/list", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		var b strings.Builder
		b.WriteString(`{"members":[`)
		for i := 0; i < f.members; i++ {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString(`{"ID":"1","name":"m"}`)
		}
		b.WriteString(`]}`)
		_, _ = w.Write([]byte(b.String()))
	})
	mux.HandleFunc("/v3/auth/authenticate", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"token":"` + f.token + `"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

func run(t *testing.T, cfg engine.EtcdConfig, target engine.EtcdTarget) []engine.Finding {
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

func TestHealthyClusterOK(t *testing.T) {
	url := fakeEtcd(t, fake{health: "true", leader: "12345", members: 3})
	fs := run(t, engine.EtcdConfig{ExpectMembers: 3}, engine.EtcdTarget{Name: "etcd", URL: url})
	f, _ := findingFor(fs, "etcd")
	if f.Status != engine.OK || !strings.Contains(f.Message, "3 members") {
		t.Fatalf("want OK with member count, got %s: %s", f.Status, f.Message)
	}
}

func TestUnhealthyIsBad(t *testing.T) {
	url := fakeEtcd(t, fake{health: "false", leader: "1", members: 3})
	f, _ := findingFor(run(t, engine.EtcdConfig{}, engine.EtcdTarget{Name: "etcd", URL: url}), "etcd")
	if f.Status != engine.BAD {
		t.Fatalf("unhealthy endpoint should be BAD, got %s: %s", f.Status, f.Message)
	}
}

func TestNoLeaderIsBad(t *testing.T) {
	url := fakeEtcd(t, fake{health: "true", leader: "0", members: 3})
	f, _ := findingFor(run(t, engine.EtcdConfig{}, engine.EtcdTarget{Name: "etcd", URL: url}), "etcd")
	if f.Status != engine.BAD || !strings.Contains(f.Message, "no leader") {
		t.Fatalf("no leader should be BAD, got %s: %s", f.Status, f.Message)
	}
}

func TestMemberShortfall(t *testing.T) {
	url := fakeEtcd(t, fake{health: "true", leader: "1", members: 2})
	fs := run(t, engine.EtcdConfig{ExpectMembers: 3}, engine.EtcdTarget{Name: "etcd", URL: url})
	m, ok := findingFor(fs, "etcd/members")
	if !ok || m.Status != engine.BAD {
		t.Fatalf("want BAD members finding for shortfall, got %+v", m)
	}
}

func TestUnreachableIsError(t *testing.T) {
	f, _ := findingFor(run(t, engine.EtcdConfig{}, engine.EtcdTarget{Name: "etcd", URL: "http://127.0.0.1:1"}), "etcd")
	if f.Status != engine.ERROR {
		t.Fatalf("unreachable should be ERROR, got %s: %s", f.Status, f.Message)
	}
}

func TestAuthTokenSent(t *testing.T) {
	url := fakeEtcd(t, fake{health: "true", leader: "1", members: 1, token: "tok-123"})
	t.Setenv("ETCD_PW", "secret")
	fs := run(t, engine.EtcdConfig{},
		engine.EtcdTarget{Name: "etcd", URL: url, Username: "root", PasswordEnv: "ETCD_PW"})
	f, _ := findingFor(fs, "etcd")
	if f.Status != engine.OK {
		t.Fatalf("authenticated call should succeed, got %s: %s", f.Status, f.Message)
	}
}
