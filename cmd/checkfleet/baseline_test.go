package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// run the CLI and report the exit code (0 when it succeeded).
func runCLI(t *testing.T, bin string, args ...string) (out string, code int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
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

// The adoption flow: first run records the debt and stays green even though
// the fleet is broken; the second run gates only on what is new.
func TestBaselineAdoptionFlow(t *testing.T) {
	bin := buildCLI(t)
	cfg := closedPortConfig(t) // one target, always ERROR
	path := filepath.Join(t.TempDir(), "baseline.json")

	// 1. First run: no baseline yet → record it, skip the gate.
	out, code := runCLI(t, bin, "check", "tcp", "--config", cfg,
		"--baseline", path, "--fail-on-new")
	if code != 0 {
		t.Fatalf("the recording run must not fail the build, got exit %d\n%s", code, out)
	}
	if !strings.Contains(out, "baseline recorded") {
		t.Errorf("expected a note that the baseline was recorded:\n%s", out)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("baseline file was not created: %v", err)
	}

	// 2. Second run, same broken fleet: the debt is known → still green.
	out, code = runCLI(t, bin, "check", "tcp", "--config", cfg,
		"--baseline", path, "--fail-on-new")
	if code != 0 {
		t.Fatalf("known debt must not fail the build, got exit %d\n%s", code, out)
	}
	if !strings.Contains(out, "0 finding(s) new or worse") {
		t.Errorf("expected zero new findings:\n%s", out)
	}

	// 3. Without the baseline the same run is red — proving the fleet really is
	//    broken and step 2's green came from the baseline, not from luck.
	if _, code = runCLI(t, bin, "check", "tcp", "--config", cfg, "--exit-on", "error"); code != defaultExitCode {
		t.Fatalf("the unfiltered run should have tripped the gate, got exit %d", code)
	}
}

// A finding the baseline never saw fails the build.
func TestBaselineFailsOnNewFinding(t *testing.T) {
	bin := buildCLI(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "baseline.json")

	// Baseline recorded against an empty-ish fleet: one target.
	cfgOne := filepath.Join(dir, "one.yml")
	writeConfig(t, cfgOne, "127.0.0.1:1")
	if _, code := runCLI(t, bin, "check", "tcp", "--config", cfgOne, "--baseline", path, "--fail-on-new"); code != 0 {
		t.Fatalf("recording run failed with %d", code)
	}

	// Now a second, previously unseen target appears and is also broken.
	cfgTwo := filepath.Join(dir, "two.yml")
	writeConfig(t, cfgTwo, "127.0.0.1:1", "127.0.0.1:2")
	out, code := runCLI(t, bin, "check", "tcp", "--config", cfgTwo, "--baseline", path, "--fail-on-new")
	if code != defaultExitCode {
		t.Fatalf("a new broken target must fail the build, got exit %d\n%s", code, out)
	}
	if !strings.Contains(out, "1 finding(s) new or worse") {
		t.Errorf("expected exactly one new finding:\n%s", out)
	}
}

// --write-baseline re-records deliberately and does not gate.
func TestBaselineWriteRefreshes(t *testing.T) {
	bin := buildCLI(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "baseline.json")

	cfgOne := filepath.Join(dir, "one.yml")
	writeConfig(t, cfgOne, "127.0.0.1:1")
	runCLI(t, bin, "check", "tcp", "--config", cfgOne, "--baseline", path, "--fail-on-new")

	cfgTwo := filepath.Join(dir, "two.yml")
	writeConfig(t, cfgTwo, "127.0.0.1:1", "127.0.0.1:2")

	// Accept the new state as the baseline...
	if out, code := runCLI(t, bin, "check", "tcp", "--config", cfgTwo,
		"--baseline", path, "--write-baseline", "--exit-on", "error"); code != 0 {
		t.Fatalf("--write-baseline must skip the gate, got exit %d\n%s", code, out)
	}
	// ...after which the same run is no longer "new".
	if out, code := runCLI(t, bin, "check", "tcp", "--config", cfgTwo,
		"--baseline", path, "--fail-on-new"); code != 0 {
		t.Fatalf("after re-recording, the run should be green, got exit %d\n%s", code, out)
	}
}

// --baseline on its own must not loosen an existing gate.
func TestBaselineWithoutFailOnNewStillGates(t *testing.T) {
	bin := buildCLI(t)
	cfg := closedPortConfig(t)
	path := filepath.Join(t.TempDir(), "baseline.json")

	runCLI(t, bin, "check", "tcp", "--config", cfg, "--baseline", path, "--write-baseline")

	out, code := runCLI(t, bin, "check", "tcp", "--config", cfg, "--baseline", path, "--exit-on", "error")
	if code != defaultExitCode {
		t.Fatalf("--baseline alone must not disable the gate, got exit %d\n%s", code, out)
	}
}

func TestBaselineFlagValidation(t *testing.T) {
	bin := buildCLI(t)
	cfg := closedPortConfig(t)

	out, code := runCLI(t, bin, "check", "tcp", "--config", cfg, "--fail-on-new")
	if code != 1 {
		t.Fatalf("--fail-on-new without --baseline should be a usage error (exit 1), got %d\n%s", code, out)
	}
	if !strings.Contains(out, "--baseline") {
		t.Errorf("the error should name the missing flag:\n%s", out)
	}
}

func TestBaselineRejectsCorruptFile(t *testing.T) {
	bin := buildCLI(t)
	cfg := closedPortConfig(t)
	path := filepath.Join(t.TempDir(), "baseline.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, code := runCLI(t, bin, "check", "tcp", "--config", cfg, "--baseline", path, "--fail-on-new")
	if code != 1 {
		t.Fatalf("a corrupt baseline is a systemic error (exit 1), got %d\n%s", code, out)
	}
}

func writeConfig(t *testing.T, path string, addrs ...string) {
	t.Helper()
	body := "timeout_seconds: 2\nchecks:\n  tcp:\n    targets:\n"
	for _, a := range addrs {
		body += "      - address: \"" + a + "\"\n"
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
