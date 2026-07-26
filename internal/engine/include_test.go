package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// write a config file in dir and return its path.
func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return p
}

// A base file's modules merge in under the including file, and the including
// file wins on shared keys (CF-115).
func TestIncludeDeepMerge(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "base.yml",
		"timeout_seconds: 99\n"+
			"checks:\n"+
			"  certs:\n"+
			"    warn_days: 30\n"+
			"    targets: [a.example:443]\n")
	main := writeFile(t, dir, "checkfleet.yml",
		"include: [base.yml]\n"+
			"timeout_seconds: 10\n"+
			"checks:\n"+
			"  certs:\n"+
			"    warn_days: 7\n"+ // overrides the include
			"  http:\n"+
			"    targets:\n"+
			"      - url: https://main.example\n")

	cfg, err := LoadConfig(main)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.TimeoutSeconds != 10 {
		t.Fatalf("timeout = %d, want 10 (main wins)", cfg.TimeoutSeconds)
	}
	if cfg.Checks.Certs == nil || cfg.Checks.HTTP == nil {
		t.Fatal("expected both certs (from include) and http (from main)")
	}
	if cfg.Checks.Certs.WarnDays != 7 {
		t.Fatalf("certs.warn_days = %d, want 7 (main overrides include)", cfg.Checks.Certs.WarnDays)
	}
	// A key only in the include is preserved through the deep merge.
	if len(cfg.Checks.Certs.Targets) != 1 || cfg.Checks.Certs.Targets[0] != "a.example:443" {
		t.Fatalf("certs.targets = %v, want the include's target kept", cfg.Checks.Certs.Targets)
	}
}

// Includes apply in listed order — a later include wins over an earlier one.
func TestIncludeOrderLastWins(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.yml", "timeout_seconds: 1\nchecks:\n  dns:\n    targets:\n      - name: a.example\n")
	writeFile(t, dir, "b.yml", "timeout_seconds: 2\nchecks:\n  http:\n    targets:\n      - url: https://b.example\n")
	main := writeFile(t, dir, "checkfleet.yml", "include: [a.yml, b.yml]\n")

	cfg, err := LoadConfig(main)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.TimeoutSeconds != 2 {
		t.Fatalf("timeout = %d, want 2 (b.yml, listed last, wins)", cfg.TimeoutSeconds)
	}
	if cfg.Checks.DNS == nil || cfg.Checks.HTTP == nil {
		t.Fatal("both a.yml (dns) and b.yml (http) modules should be present")
	}
}

// Includes nest: a → b → c, and c's module reaches the top.
func TestIncludeNested(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "c.yml", "checks:\n  dns:\n    targets:\n      - name: deep.example\n")
	writeFile(t, dir, "b.yml", "include: [c.yml]\nchecks:\n  http:\n    targets:\n      - url: https://b.example\n")
	main := writeFile(t, dir, "checkfleet.yml", "include: [b.yml]\ntimeout_seconds: 5\n")

	cfg, err := LoadConfig(main)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Checks.DNS == nil {
		t.Fatal("dns from the deepest include (c.yml) should be present")
	}
}

// An include cycle is reported, not looped forever.
func TestIncludeCycle(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.yml", "include: [b.yml]\n")
	writeFile(t, dir, "b.yml", "include: [a.yml]\n")

	_, err := LoadConfig(filepath.Join(dir, "a.yml"))
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("want a cycle error, got %v", err)
	}
}

// A missing include names the problem clearly.
func TestIncludeMissingFile(t *testing.T) {
	dir := t.TempDir()
	main := writeFile(t, dir, "checkfleet.yml", "include: [nope.yml]\n")
	_, err := LoadConfig(main)
	if err == nil {
		t.Fatal("expected an error for a missing include")
	}
}

// Interpolation still runs on an included file, on its own bytes.
func TestIncludeInterpolation(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CF_WARN", "12")
	writeFile(t, dir, "base.yml", "checks:\n  certs:\n    warn_days: ${CF_WARN:-30}\n    targets: [x:443]\n")
	main := writeFile(t, dir, "checkfleet.yml", "include: [base.yml]\n")

	cfg, err := LoadConfig(main)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Checks.Certs == nil || cfg.Checks.Certs.WarnDays != 12 {
		t.Fatalf("certs.warn_days = %v, want 12 from ${CF_WARN}", cfg.Checks.Certs)
	}
}

// Including a directory merges its *.yml drop-ins in sorted order (conf.d).
func TestIncludeConfDDirectory(t *testing.T) {
	dir := t.TempDir()
	confd := filepath.Join(dir, "conf.d")
	if err := os.Mkdir(confd, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, confd, "10-http.yml", "checks:\n  http:\n    targets:\n      - url: https://a.example\n")
	writeFile(t, confd, "20-dns.yml", "checks:\n  dns:\n    targets:\n      - name: b.example\n")
	writeFile(t, confd, "notes.txt", "ignored — not a yaml file\n")
	main := writeFile(t, dir, "checkfleet.yml", "include: [conf.d]\ntimeout_seconds: 3\n")

	cfg, err := LoadConfig(main)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Checks.HTTP == nil || cfg.Checks.DNS == nil {
		t.Fatalf("both drop-ins should be merged, got %+v", cfg.Checks)
	}
	if cfg.TimeoutSeconds != 3 {
		t.Fatalf("timeout = %d, want 3", cfg.TimeoutSeconds)
	}
}

// A plain config without any include still loads exactly as before.
func TestNoIncludeUnchanged(t *testing.T) {
	dir := t.TempDir()
	main := writeFile(t, dir, "checkfleet.yml", "timeout_seconds: 15\nchecks:\n  http:\n    targets:\n      - url: https://x.example\n")
	cfg, err := LoadConfig(main)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.TimeoutSeconds != 15 || cfg.Checks.HTTP == nil {
		t.Fatalf("plain config regressed: %+v", cfg)
	}
}
