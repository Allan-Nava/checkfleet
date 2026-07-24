package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStackPath(t *testing.T) {
	cases := map[string]string{
		"checkfleet.yml":      "checkfleet.prod.yml",
		"cfg/checkfleet.yaml": "cfg/checkfleet.prod.yaml",
		"/etc/checkfleet.yml": "/etc/checkfleet.prod.yml",
	}
	for base, want := range cases {
		if got := StackPath(base, "prod"); got != want {
			t.Errorf("StackPath(%q): want %q, got %q", base, want, got)
		}
	}
}

func TestLoadConfigStackOverlay(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "checkfleet.yml")
	if err := os.WriteFile(base, []byte(`
timeout_seconds: 10
checks:
  certs:
    warn_days: 20
    targets: [base.example]
  http:
    targets:
      - url: https://base.example/
`), 0o644); err != nil {
		t.Fatal(err)
	}
	// Stack: override certs entirely, bump timeout, leave http from base.
	if err := os.WriteFile(filepath.Join(dir, "checkfleet.prod.yml"), []byte(`
timeout_seconds: 45
checks:
  certs:
    targets: [prod.example]
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfigStack(base, "prod")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TimeoutSeconds != 45 {
		t.Errorf("timeout: the stack must win, got %d", cfg.TimeoutSeconds)
	}
	// certs replaced by the stack (base's warn_days=20 is gone → default 30).
	if got := cfg.Checks.Certs; len(got.Targets) != 1 || got.Targets[0] != "prod.example" {
		t.Errorf("certs: want stack target, got %+v", got.Targets)
	}
	if cfg.Checks.Certs.WarnDays != 30 {
		t.Errorf("certs warn_days: module replaced -> default 30, got %d", cfg.Checks.Certs.WarnDays)
	}
	// http untouched by the stack → inherited from base.
	if cfg.Checks.HTTP == nil || len(cfg.Checks.HTTP.Targets) != 1 {
		t.Errorf("http: should have stayed the base one, got %+v", cfg.Checks.HTTP)
	}
}

func TestOverlayTimeoutOnlyWhenSet(t *testing.T) {
	base := &Config{TimeoutSeconds: 10}
	base.overlay(&Config{TimeoutSeconds: 0}) // stack non imposta il timeout
	if base.TimeoutSeconds != 10 {
		t.Errorf("base timeout must not be overwritten by a stack without a timeout, got %d", base.TimeoutSeconds)
	}
}

func TestLoadConfigStackMissingFileErrors(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "checkfleet.yml")
	_ = os.WriteFile(base, []byte("checks: {certs: {targets: [x]}}\n"), 0o644)
	if _, err := LoadConfigStack(base, "assente"); err == nil {
		t.Error("nonexistent stack: want error")
	}
}
