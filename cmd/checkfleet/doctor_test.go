package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runCLIEnv is runCLI with extra environment entries.
func runCLIEnv(t *testing.T, bin string, env []string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), env...)
	b, err := cmd.CombinedOutput()
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("running %v: %v\n%s", args, err, b)
		}
		return string(b), ee.ExitCode()
	}
	return string(b), 0
}

func TestDoctorReportsUnsetVariable(t *testing.T) {
	bin := buildCLI(t)
	dir := t.TempDir()
	cfg := filepath.Join(dir, "checkfleet.yml")
	body := `timeout_seconds: 2
checks:
  postgres:
    targets:
      - {name: pg, dsn: "postgres://u:${CF_DOCTOR_ABSENT}@127.0.0.1:1/x"}
`
	if err := os.WriteFile(cfg, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	out, code := runCLIEnv(t, bin, []string{"CF_DOCTOR_ABSENT="}, "doctor", "--config", cfg, "--no-probe", "--no-color")
	if code != 0 {
		t.Fatalf("doctor must exit 0 even when it finds problems, got %d:\n%s", code, out)
	}
	if !strings.Contains(out, "CF_DOCTOR_ABSENT") {
		t.Errorf("the unset variable must be named exactly:\n%s", out)
	}
	if !strings.Contains(out, "BAD") {
		t.Errorf("an unset variable with no default should be BAD:\n%s", out)
	}
}

// The case doctor exists for: the config does not load, and the command still
// reports why instead of refusing to run.
func TestDoctorWorksOnUnloadableConfig(t *testing.T) {
	bin := buildCLI(t)
	dir := t.TempDir()
	cfg := filepath.Join(dir, "checkfleet.yml")
	// Invalid YAML *and* an unset variable: the variable must still surface.
	body := "checks:\n  http:\n    targets: [ {url: \"${CF_DOCTOR_BROKEN}\" \nbroken: [unclosed\n"
	if err := os.WriteFile(cfg, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	out, code := runCLIEnv(t, bin, []string{"CF_DOCTOR_BROKEN="}, "doctor", "--config", cfg, "--no-probe", "--no-color")
	if code != 0 {
		t.Fatalf("doctor must exit 0, got %d:\n%s", code, out)
	}
	if !strings.Contains(out, "CF_DOCTOR_BROKEN") {
		t.Errorf("the variable scan must survive an unparseable config:\n%s", out)
	}
	if !strings.Contains(out, "config") {
		t.Errorf("the load failure should be reported as a finding:\n%s", out)
	}
}

func TestDoctorProbesReachability(t *testing.T) {
	bin := buildCLI(t)
	dir := t.TempDir()
	cfg := filepath.Join(dir, "checkfleet.yml")
	// Port 1 on loopback: nothing listens, so this is a deterministic refusal
	// with no external network.
	body := "timeout_seconds: 2\nchecks:\n  tcp:\n    targets:\n      - address: \"127.0.0.1:1\"\n"
	if err := os.WriteFile(cfg, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	out, code := runCLI(t, bin, "doctor", "--config", cfg, "--no-color", "--probe-timeout", "2s")
	if code != 0 {
		t.Fatalf("doctor must exit 0, got %d:\n%s", code, out)
	}
	if !strings.Contains(out, "ERROR") || !strings.Contains(out, "127.0.0.1:1") {
		t.Errorf("expected an ERROR for the closed port:\n%s", out)
	}
	// ERROR, not BAD: we could not measure. The distinction is load-bearing.
	if strings.Contains(out, "BAD   network") {
		t.Errorf("an unreachable host is ERROR, not BAD:\n%s", out)
	}

	// --no-probe must skip it entirely.
	out, code = runCLI(t, bin, "doctor", "--config", cfg, "--no-probe", "--no-color")
	if code != 0 {
		t.Fatalf("--no-probe exited %d:\n%s", code, out)
	}
	if strings.Contains(out, "network") {
		t.Errorf("--no-probe should do no network work:\n%s", out)
	}
}

func TestDoctorJSON(t *testing.T) {
	bin := buildCLI(t)
	dir := t.TempDir()
	cfg := filepath.Join(dir, "checkfleet.yml")
	body := "timeout_seconds: 2\nchecks:\n  certs:\n    targets: [example.com]\n"
	if err := os.WriteFile(cfg, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	out, code := runCLI(t, bin, "doctor", "--config", cfg, "--no-probe", "--output", "json")
	if code != 0 {
		t.Fatalf("exited %d:\n%s", code, out)
	}
	var got struct {
		Worst    string `json:"worst"`
		Findings []struct {
			Check  string `json:"check"`
			Status string `json:"status"`
		} `json:"findings"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if len(got.Findings) == 0 {
		t.Error("no findings in the JSON output")
	}
	if got.Worst == "" {
		t.Error("the worst field should be present, as in a check run")
	}
}

func TestDoctorSystemicErrors(t *testing.T) {
	bin := buildCLI(t)
	dir := t.TempDir()
	cfg := filepath.Join(dir, "checkfleet.yml")
	if err := os.WriteFile(cfg, []byte("checks:\n  certs:\n    targets: [a.example.com]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// A missing config file is systemic: there is nothing to diagnose.
	if out, code := runCLI(t, bin, "doctor", "--config", filepath.Join(dir, "nope.yml")); code != 1 {
		t.Errorf("missing config should exit 1, got %d:\n%s", code, out)
	}
	if out, code := runCLI(t, bin, "doctor", "--config", cfg, "--output", "yaml"); code != 1 {
		t.Errorf("bad --output should exit 1, got %d:\n%s", code, out)
	}
}
