package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runCLISplit runs the binary keeping stdout and stderr apart, which is the
// point of these tests: the notice must not land in the parsed document.
func runCLISplit(t *testing.T, bin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("running %v: %v\n%s", args, err, errb.String())
		}
		return out.String(), errb.String(), ee.ExitCode()
	}
	return out.String(), errb.String(), 0
}

// A misspelled module is the failure this warning exists for: the module never
// runs, and without the notice the run reports a healthy fleet because it
// checked nothing. It must warn, and it must still exit 0 — a typo is not a
// systemic failure, and a config written for a newer checkfleet has to keep
// working on an older one (docs/compatibility.md).
func TestCheckWarnsOnUnknownKey(t *testing.T) {
	bin := buildCLI(t)
	cfg := writeCfg(t, `checks:
  tcp:
    targets:
      - {name: db, address: 127.0.0.1:1}
  postgress:
    targets:
      - {name: pg, dsn: "host=db1"}
`)

	stdout, stderr, code := runCLISplit(t, bin, "check", "all", "--config", cfg)
	if code != 0 {
		t.Fatalf("an unknown key must not change the exit code, got %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, `"postgress"`) {
		t.Errorf("stderr must name the ignored key:\n%s", stderr)
	}
	if !strings.Contains(stderr, "postgres") {
		t.Errorf("stderr must suggest the real module:\n%s", stderr)
	}
	if strings.Contains(stdout, "postgress") {
		t.Errorf("the notice belongs on stderr, not in the report:\n%s", stdout)
	}
}

// The reason stderr is not negotiable: with --output json the notice would
// otherwise make the document unparseable for the pipeline consuming it.
func TestCheckWarningKeepsJSONParseable(t *testing.T) {
	bin := buildCLI(t)
	cfg := writeCfg(t, `checks:
  tcp:
    targets:
      - {name: db, address: 127.0.0.1:1}
  htp: {}
`)

	stdout, stderr, code := runCLISplit(t, bin, "check", "all", "--config", cfg, "--output", "json")
	if code != 0 {
		t.Fatalf("exit code should be 0, got %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, `"htp"`) {
		t.Errorf("stderr must name the ignored key:\n%s", stderr)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("stdout must stay valid JSON: %v\n%s", err, stdout)
	}
	if doc["worst"] == nil {
		t.Errorf("the report itself must be intact: %v", doc)
	}
}

func TestCheckCleanConfigWarnsNothing(t *testing.T) {
	bin := buildCLI(t)
	cfg := writeCfg(t, "checks:\n  tcp:\n    targets:\n      - {name: db, address: 127.0.0.1:1}\n")

	_, stderr, code := runCLISplit(t, bin, "check", "all", "--config", cfg)
	if code != 0 {
		t.Fatalf("exit code should be 0, got %d\nstderr: %s", code, stderr)
	}
	if strings.Contains(stderr, "warning") {
		t.Errorf("a valid config must be silent, got:\n%s", stderr)
	}
}

// A typo in a stack overlay is invisible in the base config, and the overlay is
// the file people edit in a hurry.
func TestCheckWarnsOnUnknownKeyInStack(t *testing.T) {
	bin := buildCLI(t)
	dir := t.TempDir()
	base := filepath.Join(dir, "checkfleet.yml")
	if err := os.WriteFile(base, []byte("checks:\n  tcp:\n    targets:\n      - {name: db, address: 127.0.0.1:1}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	overlay := filepath.Join(dir, "checkfleet.prod.yml")
	if err := os.WriteFile(overlay, []byte("checks:\n  reds:\n    targets: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLISplit(t, bin, "check", "all", "--config", base, "--stack", "prod")
	if code != 0 {
		t.Fatalf("exit code should be 0, got %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, `"reds"`) {
		t.Errorf("stderr must name the key from the overlay:\n%s", stderr)
	}
	if !strings.Contains(stderr, "redis") {
		t.Errorf("stderr must suggest the real module:\n%s", stderr)
	}
}
