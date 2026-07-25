package vault

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

func fakeVault(t *testing.T, sealJSON string, healthCode int, healthJSON string) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/sys/seal-status", func(w http.ResponseWriter, r *http.Request) {
		if sealJSON == "" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(sealJSON))
	})
	mux.HandleFunc("/v1/sys/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(healthCode) // Vault uses non-200 for standby etc.
		_, _ = w.Write([]byte(healthJSON))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

func run(t *testing.T, target engine.VaultTarget) engine.Finding {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return New(engine.VaultConfig{}).probe(ctx, target)
}

func TestActiveUnsealedOK(t *testing.T) {
	url := fakeVault(t,
		`{"sealed":false,"initialized":true,"t":3,"n":5,"progress":0,"version":"1.15.6"}`,
		200, `{"sealed":false,"standby":false,"version":"1.15.6"}`)
	f := run(t, engine.VaultTarget{Name: "vault", URL: url})
	if f.Status != engine.OK || !strings.Contains(f.Message, "active") || !strings.Contains(f.Message, "1.15.6") {
		t.Fatalf("want OK active with version, got %s: %s", f.Status, f.Message)
	}
}

func TestStandbyIsOK(t *testing.T) {
	url := fakeVault(t,
		`{"sealed":false,"initialized":true,"version":"1.15.6"}`,
		429, `{"sealed":false,"standby":true,"version":"1.15.6"}`)
	f := run(t, engine.VaultTarget{Name: "vault", URL: url})
	if f.Status != engine.OK || !strings.Contains(f.Message, "standby") {
		t.Fatalf("standby should be OK, got %s: %s", f.Status, f.Message)
	}
}

func TestSealedIsBad(t *testing.T) {
	url := fakeVault(t,
		`{"sealed":true,"initialized":true,"t":3,"n":5,"progress":1}`,
		503, `{"sealed":true,"standby":false}`)
	f := run(t, engine.VaultTarget{Name: "vault", URL: url})
	if f.Status != engine.BAD || !strings.Contains(f.Message, "sealed") || !strings.Contains(f.Message, "1/3") {
		t.Fatalf("sealed should be BAD with progress, got %s: %s", f.Status, f.Message)
	}
}

func TestUninitializedIsBad(t *testing.T) {
	url := fakeVault(t, `{"sealed":true,"initialized":false}`, 501, `{}`)
	f := run(t, engine.VaultTarget{Name: "vault", URL: url})
	if f.Status != engine.BAD || !strings.Contains(f.Message, "not initialized") {
		t.Fatalf("uninitialized should be BAD, got %s: %s", f.Status, f.Message)
	}
}

func TestUnreachableIsError(t *testing.T) {
	f := run(t, engine.VaultTarget{Name: "vault", URL: "http://127.0.0.1:1"})
	if f.Status != engine.ERROR {
		t.Fatalf("unreachable should be ERROR, got %s: %s", f.Status, f.Message)
	}
}
