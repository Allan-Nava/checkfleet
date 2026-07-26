package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSplitCSV(t *testing.T) {
	cases := map[string][]string{
		"":               nil,
		"text":           {"text"},
		"text,slack":     {"text", "slack"},
		" text , slack ": {"text", "slack"},
		"json,,webhook,": {"json", "webhook"},
		",":              nil,
	}
	for in, want := range cases {
		got := splitCSV(in)
		if len(got) != len(want) {
			t.Fatalf("splitCSV(%q) = %v, want %v", in, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("splitCSV(%q)[%d] = %q, want %q", in, i, got[i], want[i])
			}
		}
	}
}

// build the checkfleet binary once for the integration tests.
func buildCLI(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "checkfleet")
	out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput()
	if err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return bin
}

// A closed local port gives a deterministic ERROR finding with no real network.
func closedPortConfig(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "checkfleet.yml")
	body := "timeout_seconds: 2\nchecks:\n  tcp:\n    targets:\n      - address: \"127.0.0.1:1\"\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// --output json,webhook fans out: stdout gets the JSON AND the webhook is POSTed.
func TestFanOutHitsEverySink(t *testing.T) {
	bin := buildCLI(t)
	cfg := closedPortConfig(t)

	var got int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&got, 1)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	cmd := exec.Command(bin, "check", "tcp", "--config", cfg, "--output", "json,webhook", "--webhook-env", "CF_HOOK")
	cmd.Env = append(os.Environ(), "CF_HOOK="+srv.URL)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), `"check": "tcp"`) {
		t.Fatalf("stdout missing the json sink:\n%s", out)
	}
	if atomic.LoadInt32(&got) == 0 {
		t.Fatal("webhook sink was never POSTed")
	}
}

// An unconfigured sink in a fan-out is isolated: the run still succeeds (exit 0)
// and the other sinks still fire; the failure is reported on stderr.
func TestFanOutIsolatesFailingSink(t *testing.T) {
	bin := buildCLI(t)
	cfg := closedPortConfig(t)

	// CF_HOOK deliberately unset → the webhook sink can't run.
	cmd := exec.Command(bin, "check", "tcp", "--config", cfg, "--output", "json,webhook", "--webhook-env", "CF_HOOK")
	cmd.Env = append(os.Environ(), "CF_HOOK=")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fan-out must not fail on one bad sink, got: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), `"check": "tcp"`) {
		t.Fatalf("the json sink should still have run:\n%s", out)
	}
	if !strings.Contains(string(out), `output "webhook"`) {
		t.Fatalf("stderr should name the failing sink:\n%s", out)
	}
}
