package main

// Tests for the forge-backed commands (CF-157).
//
// `report-issues` drives `gh`/`glab` as subprocesses, so it is testable offline
// without mocking anything: put a fake executable first on PATH and read back
// what it was asked to do. No token, no network, no real tracker.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeForge installs an executable named cmd on PATH that logs its arguments to
// a file and answers `issue list` with an empty JSON array. It returns the log
// path.
func fakeForge(t *testing.T, cmd string) string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")
	script := "#!/bin/sh\necho \"$@\" >> " + log + "\n" +
		// Both CLIs are asked for a JSON list of open issues; nothing is open.
		"case \"$*\" in *list*) echo '[]' ;; esac\nexit 0\n"
	path := filepath.Join(dir, cmd)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

func calls(t *testing.T, log string) string {
	t.Helper()
	b, err := os.ReadFile(log)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatal(err)
	}
	return string(b)
}

// An unreachable target is ERROR, which report-issues treats as worth an issue:
// the run opens one, and the label is created first so the issue can carry it.
func TestReportIssuesOpensViaGH(t *testing.T) {
	log := fakeForge(t, "gh")
	if err := runReportIssues([]string{"--config", offlineConfig(t)}); err != nil {
		t.Fatal(err)
	}
	got := calls(t, log)
	if !strings.Contains(got, "issue list") {
		t.Errorf("it must read the open issues before deciding: %q", got)
	}
	if !strings.Contains(got, "issue create") {
		t.Errorf("an ERROR finding should open an issue: %q", got)
	}
	if !strings.Contains(got, "label create") {
		t.Errorf("the label must be ensured before creating: %q", got)
	}
}

// --dry-run is the flag people try first on a real repo. It must touch nothing,
// including the label.
func TestReportIssuesDryRunTouchesNothing(t *testing.T) {
	log := fakeForge(t, "gh")
	if err := runReportIssues([]string{"--config", offlineConfig(t), "--dry-run"}); err != nil {
		t.Fatal(err)
	}
	got := calls(t, log)
	if strings.Contains(got, "issue create") || strings.Contains(got, "label create") {
		t.Errorf("--dry-run must not change anything, but ran: %q", got)
	}
}

func TestReportIssuesViaGitLab(t *testing.T) {
	log := fakeForge(t, "glab")
	if err := runReportIssues([]string{"--config", offlineConfig(t), "--forge", "gitlab"}); err != nil {
		t.Fatal(err)
	}
	if got := calls(t, log); !strings.Contains(got, "issue create") {
		t.Errorf("the gitlab adapter should open an issue: %q", got)
	}
}

func TestReportIssuesUnknownForge(t *testing.T) {
	err := runReportIssues([]string{"--config", offlineConfig(t), "--forge", "bitbucket"})
	if err == nil || !strings.Contains(err.Error(), "bitbucket") {
		t.Errorf("an unknown forge must be a systemic error naming it, got %v", err)
	}
}

// --- scaffolding from an inventory ------------------------------------------

func TestInitFromInventoryInProcess(t *testing.T) {
	dir := t.TempDir()
	inv := filepath.Join(dir, "hosts.ini")
	if err := os.WriteFile(inv, []byte("[web]\nweb1 ansible_host=10.0.0.1\nweb2\n\n[db]\ndb1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(dir, "checkfleet.yml")

	if err := runInit([]string{"--config", cfg, "--from-inventory", inv, "--modules", "tcp", "--group", "web"}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	body := string(b)
	// --group restricts the inventory side: db1 is not in `web`.
	if strings.Contains(body, "db1") {
		t.Errorf("--group web must not pull in hosts from other groups:\n%s", body)
	}
	// ansible_host wins where it is set, since that is the address that works.
	if !strings.Contains(body, "10.0.0.1") || !strings.Contains(body, "web2") {
		t.Errorf("both hosts should be present, addressed as the inventory says:\n%s", body)
	}
	// Whatever it generated has to be something checkfleet itself accepts.
	if err := runValidate([]string{"--config", cfg}); err != nil {
		t.Errorf("a config scaffolded from an inventory must validate: %v", err)
	}
}

// A module whose target cannot be derived from a hostname alone is refused with
// an explanation, rather than scaffolded with an invented DSN that fails on the
// first run.
func TestInitFromInventoryRefusesUnderivableModuleInProcess(t *testing.T) {
	dir := t.TempDir()
	inv := filepath.Join(dir, "hosts.ini")
	if err := os.WriteFile(inv, []byte("[db]\ndb1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runInit([]string{"--config", filepath.Join(dir, "checkfleet.yml"), "--from-inventory", inv, "--modules", "postgres"})
	if err == nil || !strings.Contains(err.Error(), "postgres") {
		t.Errorf("postgres needs a DSN and must be refused by name, got %v", err)
	}
}

func TestUsageListsEveryCommand(t *testing.T) {
	// usage() writes to stderr; capturing it keeps the test output clean and
	// asserts the text a user sees when they get the invocation wrong.
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	usage()
	_ = w.Close()
	os.Stderr = orig

	b := make([]byte, 8192)
	n, _ := r.Read(b)
	text := string(b[:n])
	for _, cmd := range []string{"init", "check", "serve", "validate", "doctor", "targets", "explain", "completion", "version", "report-issues", "alert"} {
		if !strings.Contains(text, cmd) {
			t.Errorf("usage does not mention %q — a command nobody can discover:\n%s", cmd, text)
		}
	}
}

// --- alert ------------------------------------------------------------------

// The senders POST to hardcoded provider endpoints (events.pagerduty.com,
// api.opsgenie.com, sns.<region>.amazonaws.com), so only the planning half is
// testable offline — which is where the decisions are. The payloads themselves
// are covered by internal/alert.
func TestAlertDryRunSendsNothing(t *testing.T) {
	cfg := offlineConfig(t)
	if err := runAlert([]string{"--config", cfg, "--provider", "pagerduty", "--dry-run"}); err != nil {
		t.Errorf("--dry-run must work without a key: %v", err)
	}
}

// With --history, a target that recovered since the previous run gets resolved.
// The plan has to be computed from the history, so the path must survive a
// history file that exists and one that does not.
func TestAlertWithHistory(t *testing.T) {
	cfg := offlineConfig(t)
	hist := filepath.Join(t.TempDir(), "h.jsonl")
	if err := runAlert([]string{"--config", cfg, "--history", hist, "--dry-run"}); err != nil {
		t.Errorf("a missing history is not an error: %v", err)
	}
	if err := runCheck([]string{"all", "--config", cfg, "--history", hist, "--output", "csv"}); err != nil {
		t.Fatal(err)
	}
	if err := runAlert([]string{"--config", cfg, "--history", hist, "--dry-run"}); err != nil {
		t.Errorf("with a history recorded: %v", err)
	}
}

func TestAlertRejectsUnknownProvider(t *testing.T) {
	err := runAlert([]string{"--config", offlineConfig(t), "--provider", "carrier-pigeon", "--dry-run"})
	if err == nil || !strings.Contains(err.Error(), "carrier-pigeon") {
		t.Errorf("an unknown provider must be named in the error, got %v", err)
	}
}

// Without a key there is nothing to authenticate with: it must fail before
// sending rather than post an unauthenticated event.
func TestAlertRequiresKey(t *testing.T) {
	t.Setenv("EMPTY_KEY", "")
	err := runAlert([]string{"--config", offlineConfig(t), "--provider", "pagerduty", "--key-env", "EMPTY_KEY"})
	if err == nil || !strings.Contains(err.Error(), "EMPTY_KEY") {
		t.Errorf("a missing key must be reported by env var name, got %v", err)
	}
}
