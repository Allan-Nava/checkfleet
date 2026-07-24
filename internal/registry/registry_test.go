package registry

import (
	"testing"

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
