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

// Multiple stacks compose left-to-right: the last one wins (CF-117).
func TestLoadConfigStacksComposeLastWins(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "checkfleet.yml")
	writeFile(t, dir, "checkfleet.yml",
		"timeout_seconds: 5\nchecks:\n  http:\n    targets:\n      - url: https://base/\n")
	// region overrides http; env overrides http again + adds dns and a timeout.
	writeFile(t, dir, "checkfleet.region.yml",
		"checks:\n  http:\n    targets:\n      - url: https://region/\n")
	writeFile(t, dir, "checkfleet.env.yml",
		"timeout_seconds: 20\nchecks:\n  http:\n    targets:\n      - url: https://env/\n  dns:\n    targets:\n      - name: env.example\n")

	cfg, err := LoadConfigStacks(base, []string{"region", "env"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TimeoutSeconds != 20 {
		t.Fatalf("timeout = %d, want 20 (env, last, wins)", cfg.TimeoutSeconds)
	}
	if cfg.Checks.HTTP == nil || cfg.Checks.HTTP.Targets[0].URL != "https://env/" {
		t.Fatalf("http should be env's (last wins), got %+v", cfg.Checks.HTTP)
	}
	if cfg.Checks.DNS == nil {
		t.Fatal("dns from the env stack should be present")
	}
}

// A stack can override a module the old hand-listed overlay ignored (e.g.
// redis): the reflection-based overlay covers every module.
func TestOverlayCoversAllModules(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "checkfleet.yml")
	writeFile(t, dir, "checkfleet.yml",
		"checks:\n  redis:\n    targets: [base-redis:6379]\n")
	writeFile(t, dir, "checkfleet.prod.yml",
		"checks:\n  redis:\n    targets: [prod-redis:6379]\n")

	cfg, err := LoadConfigStack(base, "prod")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Checks.Redis == nil || len(cfg.Checks.Redis.Targets) != 1 || cfg.Checks.Redis.Targets[0] != "prod-redis:6379" {
		t.Fatalf("redis should be overridden by the stack, got %+v", cfg.Checks.Redis)
	}
}

// Empty entries in the stack list are ignored (so "prod," or "" is harmless).
func TestLoadConfigStacksIgnoresEmpty(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "checkfleet.yml")
	writeFile(t, dir, "checkfleet.yml", "timeout_seconds: 7\nchecks:\n  http:\n    targets:\n      - url: https://base/\n")
	cfg, err := LoadConfigStacks(base, []string{"", ""})
	if err != nil {
		t.Fatalf("empty stacks should be a no-op, got %v", err)
	}
	if cfg.TimeoutSeconds != 7 {
		t.Fatalf("timeout = %d, want base 7", cfg.TimeoutSeconds)
	}
}
