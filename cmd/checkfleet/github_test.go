package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The github sink writes annotations to stdout AND appends the report to the
// job summary file, so no shell pipe is needed to get both.
func TestGitHubSinkWritesAnnotationsAndSummary(t *testing.T) {
	bin := buildCLI(t)
	cfg := closedPortConfig(t) // a closed local port → deterministic ERROR

	summary := filepath.Join(t.TempDir(), "summary.md")
	cmd := exec.Command(bin, "check", "tcp", "--config", cfg, "--output", "github")
	cmd.Env = append(os.Environ(), "GITHUB_STEP_SUMMARY="+summary)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run failed: %v\n%s", err, out)
	}

	if !strings.Contains(string(out), "::error title=checkfleet tcp") {
		t.Errorf("stdout is missing the workflow command annotation:\n%s", out)
	}
	// 127.0.0.1:1 — the colon must be escaped inside the title property.
	if !strings.Contains(string(out), "127.0.0.1%3A1") {
		t.Errorf("the colon in the target was not escaped in the annotation property:\n%s", out)
	}

	body, err := os.ReadFile(summary)
	if err != nil {
		t.Fatalf("job summary was not written: %v", err)
	}
	if !strings.Contains(string(body), "# checkfleet — tcp") {
		t.Errorf("job summary is not the markdown report:\n%s", body)
	}
}

// $GITHUB_STEP_SUMMARY is shared with the other steps of the job, so the sink
// must append rather than replace.
func TestGitHubSinkAppendsToExistingSummary(t *testing.T) {
	bin := buildCLI(t)
	cfg := closedPortConfig(t)

	summary := filepath.Join(t.TempDir(), "summary.md")
	if err := os.WriteFile(summary, []byte("## written by an earlier step\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(bin, "check", "tcp", "--config", cfg, "--output", "github")
	cmd.Env = append(os.Environ(), "GITHUB_STEP_SUMMARY="+summary)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run failed: %v\n%s", err, out)
	}

	body, err := os.ReadFile(summary)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "written by an earlier step") {
		t.Errorf("the earlier step's summary was clobbered:\n%s", body)
	}
	if !strings.Contains(string(body), "# checkfleet — tcp") {
		t.Errorf("our report was not appended:\n%s", body)
	}
}

// Outside Actions there is no summary file; the sink must still emit its
// annotations and must not fail the run.
func TestGitHubSinkOutsideActions(t *testing.T) {
	bin := buildCLI(t)
	cfg := closedPortConfig(t)

	cmd := exec.Command(bin, "check", "tcp", "--config", cfg, "--output", "github")
	cmd.Env = append(os.Environ(), "GITHUB_STEP_SUMMARY=")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run must not fail outside Actions: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "::error title=checkfleet tcp") {
		t.Errorf("annotations should still go to stdout:\n%s", out)
	}
}

// The gate is independent of the sink: --output github --exit-on error must
// still exit non-zero, without any shell pipe in between.
func TestGitHubSinkWithGate(t *testing.T) {
	bin := buildCLI(t)
	cfg := closedPortConfig(t)

	summary := filepath.Join(t.TempDir(), "summary.md")
	cmd := exec.Command(bin, "check", "tcp", "--config", cfg, "--output", "github", "--exit-on", "error")
	cmd.Env = append(os.Environ(), "GITHUB_STEP_SUMMARY="+summary)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("--exit-on error should have tripped on an ERROR finding:\n%s", out)
	}
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != defaultExitCode {
		t.Fatalf("want exit %d, got %v", defaultExitCode, err)
	}
	// The report must still have been written before the process exited.
	if body, rerr := os.ReadFile(summary); rerr != nil || !strings.Contains(string(body), "# checkfleet — tcp") {
		t.Errorf("summary missing after a tripped gate (err=%v):\n%s", rerr, body)
	}
}
