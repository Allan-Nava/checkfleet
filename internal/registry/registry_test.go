package registry

import (
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

func TestNamesAndConfigured(t *testing.T) {
	cfg := &engine.Config{}
	cfg.Checks.Certs = &engine.CertsConfig{}
	cfg.Checks.DNS = &engine.DNSConfig{}

	names := Names(cfg)
	if len(names) != 2 {
		t.Fatalf("Names = %v, want 2 configured", names)
	}
	// Registry order: certs comes before dns.
	if names[0] != "certs" || names[1] != "dns" {
		t.Fatalf("Names = %v, want [certs dns] in registry order", names)
	}

	if got := len(Configured(cfg)); got != 2 {
		t.Fatalf("Configured built %d checks, want 2", got)
	}
}

func TestAllListsEveryModule(t *testing.T) {
	cfg := &engine.Config{} // nothing configured
	all := All(cfg)
	if len(all) != len(Modules(cfg)) || len(all) == 0 {
		t.Fatalf("All = %v, want every known module", all)
	}
	if len(Configured(cfg)) != 0 {
		t.Fatalf("Configured on empty config should be empty")
	}
	// All must be stable and contain a known module.
	seen := map[string]bool{}
	for _, n := range all {
		if seen[n] {
			t.Fatalf("duplicate module name %q in All", n)
		}
		seen[n] = true
	}
	if !seen["certs"] {
		t.Fatalf("All missing 'certs': %v", all)
	}
}

func TestOptionsForOverride(t *testing.T) {
	base := engine.Options{Timeout: 30 * time.Second, Retries: 0, Backoff: 500 * time.Millisecond}
	cfg := &engine.Config{ModuleOverrides: map[string]engine.ModuleOverride{
		"postgres": {TimeoutSeconds: 10, Retries: 2},
	}}
	// Module with an override: timeout+retries win, backoff falls back to base.
	pg := OptionsFor(cfg, "postgres", base)
	if pg.Timeout != 10*time.Second || pg.Retries != 2 || pg.Backoff != 500*time.Millisecond {
		t.Errorf("postgres override wrong: %+v", pg)
	}
	// Module without an override: base unchanged.
	if got := OptionsFor(cfg, "http", base); got != base {
		t.Errorf("http should keep base, got %+v", got)
	}
}

func TestJobsAppliesOverrides(t *testing.T) {
	base := engine.Options{Timeout: 30 * time.Second}
	cfg := &engine.Config{
		Checks: engine.ChecksConfig{
			HTTP:  &engine.HTTPConfig{Targets: []engine.HTTPTarget{{URL: "https://x/"}}},
			Certs: &engine.CertsConfig{Targets: []string{"x:443"}},
		},
		ModuleOverrides: map[string]engine.ModuleOverride{"certs": {TimeoutSeconds: 5}},
	}
	jobs := Jobs(cfg, base)
	if len(jobs) != 2 {
		t.Fatalf("want 2 jobs, got %d", len(jobs))
	}
	// Jobs follow registry order (certs before http); certs got the override.
	if jobs[0].Opts.Timeout != 5*time.Second {
		t.Errorf("certs job should have 5s timeout, got %s", jobs[0].Opts.Timeout)
	}
	if jobs[1].Opts.Timeout != 30*time.Second {
		t.Errorf("http job should keep base 30s, got %s", jobs[1].Opts.Timeout)
	}
}
