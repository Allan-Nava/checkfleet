package keycloak

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// fakeKeycloak serves /health/ready and per-realm OIDC discovery. The issuer is
// derived from the server's own URL so it's coherent by default.
func fakeKeycloak(t *testing.T, healthStatus string, realms map[string]bool) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		if healthStatus == "" {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		fmt.Fprintf(w, `{"status":%q}`, healthStatus)
	})
	mux.HandleFunc("/realms/", func(w http.ResponseWriter, r *http.Request) {
		// /realms/<name>/.well-known/openid-configuration
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/realms/"), "/")
		realm := parts[0]
		if !realms[realm] {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, `{"issuer":"%s/realms/%s","token_endpoint":"%s/realms/%s/protocol/openid-connect/token"}`,
			srv.URL, realm, srv.URL, realm)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func run(t *testing.T, cfg engine.KeycloakConfig) map[string]engine.Finding {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	m := map[string]engine.Finding{}
	for _, f := range New(cfg).Run(ctx) {
		m[f.Target] = f
	}
	return m
}

func TestHealthyAndRealm(t *testing.T) {
	srv := fakeKeycloak(t, "UP", map[string]bool{"main": true})
	f := run(t, engine.KeycloakConfig{BaseURL: srv.URL, HealthURL: srv.URL + "/health/ready", Realms: []string{"main"}})
	if f["health"].Status != engine.OK {
		t.Errorf("health: want OK, got %s (%s)", f["health"].Status, f["health"].Message)
	}
	if f["realm/main"].Status != engine.OK {
		t.Errorf("realm: want OK, got %s (%s)", f["realm/main"].Status, f["realm/main"].Message)
	}
}

func TestHealthDown(t *testing.T) {
	srv := fakeKeycloak(t, "DOWN", nil)
	f := run(t, engine.KeycloakConfig{BaseURL: srv.URL, HealthURL: srv.URL + "/health/ready"})
	if f["health"].Status != engine.BAD {
		t.Errorf("health DOWN: want BAD, got %s (%s)", f["health"].Status, f["health"].Message)
	}
}

func TestMissingRealmIsBad(t *testing.T) {
	srv := fakeKeycloak(t, "UP", map[string]bool{"main": true})
	f := run(t, engine.KeycloakConfig{BaseURL: srv.URL, Realms: []string{"missing"}})
	if f["realm/missing"].Status != engine.BAD {
		t.Errorf("missing realm: want BAD, got %s (%s)", f["realm/missing"].Status, f["realm/missing"].Message)
	}
}

func TestIssuerDriftIsWarn(t *testing.T) {
	// Server whose discovery issuer points to a different host (proxy misconfig).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"issuer":"https://wrong.example/realms/main","token_endpoint":"https://wrong.example/t"}`)
	}))
	t.Cleanup(srv.Close)
	f := run(t, engine.KeycloakConfig{BaseURL: srv.URL, Realms: []string{"other"}})
	if f["realm/other"].Status != engine.WARN || !strings.Contains(f["realm/other"].Message, "issuer") {
		t.Errorf("issuer drift: want WARN, got %s (%s)", f["realm/other"].Status, f["realm/other"].Message)
	}
}

func TestUnreachableIsError(t *testing.T) {
	f := run(t, engine.KeycloakConfig{BaseURL: "http://127.0.0.1:1", Realms: []string{"main"}})
	if f["realm/main"].Status != engine.ERROR {
		t.Errorf("unreachable: want ERROR, got %s (%s)", f["realm/main"].Status, f["realm/main"].Message)
	}
}
