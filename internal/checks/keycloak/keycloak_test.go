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
	srv := fakeKeycloak(t, "UP", map[string]bool{"hiway": true})
	f := run(t, engine.KeycloakConfig{BaseURL: srv.URL, HealthURL: srv.URL + "/health/ready", Realms: []string{"hiway"}})
	if f["health"].Status != engine.OK {
		t.Errorf("health: atteso OK, avuto %s (%s)", f["health"].Status, f["health"].Message)
	}
	if f["realm/hiway"].Status != engine.OK {
		t.Errorf("realm: atteso OK, avuto %s (%s)", f["realm/hiway"].Status, f["realm/hiway"].Message)
	}
}

func TestHealthDown(t *testing.T) {
	srv := fakeKeycloak(t, "DOWN", nil)
	f := run(t, engine.KeycloakConfig{BaseURL: srv.URL, HealthURL: srv.URL + "/health/ready"})
	if f["health"].Status != engine.BAD {
		t.Errorf("health DOWN: atteso BAD, avuto %s (%s)", f["health"].Status, f["health"].Message)
	}
}

func TestMissingRealmIsBad(t *testing.T) {
	srv := fakeKeycloak(t, "UP", map[string]bool{"hiway": true})
	f := run(t, engine.KeycloakConfig{BaseURL: srv.URL, Realms: []string{"assente"}})
	if f["realm/assente"].Status != engine.BAD {
		t.Errorf("realm assente: atteso BAD, avuto %s (%s)", f["realm/assente"].Status, f["realm/assente"].Message)
	}
}

func TestIssuerDriftIsWarn(t *testing.T) {
	// Server whose discovery issuer points to a different host (proxy misconfig).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"issuer":"https://wrong.example/realms/hiway","token_endpoint":"https://wrong.example/t"}`)
	}))
	t.Cleanup(srv.Close)
	f := run(t, engine.KeycloakConfig{BaseURL: srv.URL, Realms: []string{"altro"}})
	if f["realm/altro"].Status != engine.WARN || !strings.Contains(f["realm/altro"].Message, "issuer") {
		t.Errorf("issuer drift: atteso WARN, avuto %s (%s)", f["realm/altro"].Status, f["realm/altro"].Message)
	}
}

func TestUnreachableIsError(t *testing.T) {
	f := run(t, engine.KeycloakConfig{BaseURL: "http://127.0.0.1:1", Realms: []string{"hiway"}})
	if f["realm/hiway"].Status != engine.ERROR {
		t.Errorf("irraggiungibile: atteso ERROR, avuto %s (%s)", f["realm/hiway"].Status, f["realm/hiway"].Message)
	}
}
