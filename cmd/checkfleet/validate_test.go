package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeCfg(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "checkfleet.yml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestValidateNamesTypoAndSuggestsFix(t *testing.T) {
	bin := buildCLI(t)
	cfg := writeCfg(t, "checks:\n  postgress:\n    targets:\n      - {name: pg, dsn: \"host=db1\"}\n")

	out, code := runCLI(t, bin, "validate", "--config", cfg)
	if code != 1 {
		t.Fatalf("a config whose module is misspelled must fail, got %d:\n%s", code, out)
	}
	if !strings.Contains(out, `"postgress"`) {
		t.Errorf("the output must name the misspelled key:\n%s", out)
	}
	if !strings.Contains(out, "postgres") {
		t.Errorf("the output must suggest the right one:\n%s", out)
	}
}

// An unset variable is a note, not a failure: validate is documented for
// pre-commit hooks, where production secrets are not exported.
func TestValidateDoesNotFailOnUnsetVariable(t *testing.T) {
	bin := buildCLI(t)
	cfg := writeCfg(t, `checks:
  postgres:
    targets:
      - {name: pg, dsn: "postgres://u:${CF_VALIDATE_ABSENT}@db1:5432/x"}
`)

	out, code := runCLIEnv(t, bin, []string{"CF_VALIDATE_ABSENT="}, "validate", "--config", cfg)
	if code != 0 {
		t.Fatalf("an unset variable must not fail validate, got %d:\n%s", code, out)
	}
	if !strings.Contains(out, "is valid") {
		t.Errorf("the config itself is valid, so say so:\n%s", out)
	}
	if !strings.Contains(out, "note:") || !strings.Contains(out, "CF_VALIDATE_ABSENT") {
		t.Errorf("the unset variable should still be reported as a note:\n%s", out)
	}
}

// A config using the documented include: feature must stay valid.
func TestValidateAcceptsInclude(t *testing.T) {
	bin := buildCLI(t)
	dir := t.TempDir()
	inc := filepath.Join(dir, "extra.yml")
	if err := os.WriteFile(inc, []byte("checks:\n  http:\n    targets: [{url: \"https://example.com/\"}]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(dir, "checkfleet.yml")
	if err := os.WriteFile(cfg, []byte("include: extra.yml\nchecks:\n  certs:\n    targets: [example.com]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, code := runCLI(t, bin, "validate", "--config", cfg)
	if code != 0 {
		t.Fatalf("include is a valid key, got exit %d:\n%s", code, out)
	}
}

// A config that cannot be loaded should still be told why, with the raw-level
// problems that usually explain it.
func TestValidateOnUnloadableConfig(t *testing.T) {
	bin := buildCLI(t)
	cfg := writeCfg(t, "checks:\n  http:\n    targets: [ {url: \"x\" \nbroken: [unclosed\n")

	out, code := runCLI(t, bin, "validate", "--config", cfg)
	if code != 1 {
		t.Fatalf("want exit 1, got %d:\n%s", code, out)
	}
	if !strings.Contains(out, "cannot be loaded") {
		t.Errorf("the output should say the config could not be loaded:\n%s", out)
	}
}

func TestValidateCleanConfig(t *testing.T) {
	bin := buildCLI(t)
	cfg := writeCfg(t, "timeout_seconds: 5\nchecks:\n  certs:\n    warn_days: 30\n    crit_days: 7\n    targets: [example.com]\n")

	out, code := runCLI(t, bin, "validate", "--config", cfg)
	if code != 0 {
		t.Fatalf("a clean config must exit 0, got %d:\n%s", code, out)
	}
	if !strings.Contains(out, "is valid") {
		t.Errorf("unexpected output:\n%s", out)
	}
	if strings.Contains(out, "note:") {
		t.Errorf("a clean config should have no notes:\n%s", out)
	}
}
