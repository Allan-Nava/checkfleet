package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/coverage"
)

func TestFormatTargetsGroupsByModule(t *testing.T) {
	targets := []coverage.Target{
		{Module: "certs", Name: "github.com", Hosts: []string{"github.com"}},
		{Module: "certs", Name: "api.example.com", Hosts: []string{"api.example.com"}},
		{Module: "http", Name: "https://example.com/health", Hosts: []string{"example.com"}},
	}
	out := formatTargets(targets, nil, "")

	if !strings.Contains(out, "3 target(s) across 2 module(s)") {
		t.Errorf("missing the summary line:\n%s", out)
	}
	if !strings.Contains(out, "certs (2)") || !strings.Contains(out, "http (1)") {
		t.Errorf("missing per-module counts:\n%s", out)
	}
	// The host column appears only when it adds information.
	if strings.Contains(out, "github.com                                            → github.com") {
		t.Errorf("host repeated when identical to the name:\n%s", out)
	}
	if !strings.Contains(out, "https://example.com/health") || !strings.Contains(out, "→ example.com") {
		t.Errorf("expected the extracted host next to the URL:\n%s", out)
	}
}

func TestFormatTargetsCoverageSections(t *testing.T) {
	targets := []coverage.Target{{Module: "http", Name: "web1", Hosts: []string{"web1"}}}
	d := &coverage.Diff{
		Covered:   []string{"web1"},
		Uncovered: []string{"db1"},
		Extra:     []string{"github.com"},
	}
	out := formatTargets(targets, d, "hosts.ini")

	for _, want := range []string{
		"coverage vs hosts.ini: 1/2 inventory host(s) covered",
		"not monitored (1)",
		"db1",
		"targeted but not in the inventory (1)",
		"github.com",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestFormatTargetsAllCovered(t *testing.T) {
	d := &coverage.Diff{Covered: []string{"web1", "web2"}}
	out := formatTargets(nil, d, "hosts.ini")
	if !strings.Contains(out, "every inventory host is covered") {
		t.Errorf("expected the all-clear line:\n%s", out)
	}
	if strings.Contains(out, "not monitored") {
		t.Errorf("no 'not monitored' section should appear:\n%s", out)
	}
}

func writeTargetsConfig(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "checkfleet.yml")
	body := `timeout_seconds: 5
checks:
  certs:
    targets: [web1.example.com, github.com]
  http:
    targets:
      - url: https://web2.example.com/health
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTargetsCommandTextAndJSON(t *testing.T) {
	bin := buildCLI(t)
	dir := t.TempDir()
	cfg := writeTargetsConfig(t, dir)

	out, code := runCLI(t, bin, "targets", "--config", cfg)
	if code != 0 {
		t.Fatalf("targets exited %d:\n%s", code, out)
	}
	if !strings.Contains(out, "3 target(s) across 2 module(s)") {
		t.Errorf("unexpected text output:\n%s", out)
	}

	out, code = runCLI(t, bin, "targets", "--config", cfg, "--output", "json")
	if code != 0 {
		t.Fatalf("targets --output json exited %d:\n%s", code, out)
	}
	var got struct {
		Targets []coverage.Target `json:"targets"`
		Diff    *coverage.Diff    `json:"coverage"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if len(got.Targets) != 3 {
		t.Errorf("got %d targets, want 3", len(got.Targets))
	}
	// Without --against there is no coverage object at all.
	if got.Diff != nil {
		t.Errorf("coverage should be absent without --against: %+v", got.Diff)
	}
}

func TestTargetsCommandAgainstInventory(t *testing.T) {
	bin := buildCLI(t)
	dir := t.TempDir()
	cfg := writeTargetsConfig(t, dir)

	inv := filepath.Join(dir, "hosts.ini")
	body := "[web]\nweb1.example.com\nweb2.example.com\n\n[db]\ndb1.example.com\n"
	if err := os.WriteFile(inv, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	out, code := runCLI(t, bin, "targets", "--config", cfg, "--against", inv, "--output", "json")
	if code != 0 {
		t.Fatalf("exited %d:\n%s", code, out)
	}
	var got struct {
		Diff *coverage.Diff `json:"coverage"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if got.Diff == nil {
		t.Fatal("coverage missing with --against")
	}
	if len(got.Diff.Covered) != 2 {
		t.Errorf("covered = %v, want web1 and web2", got.Diff.Covered)
	}
	if len(got.Diff.Uncovered) != 1 || got.Diff.Uncovered[0] != "db1.example.com" {
		t.Errorf("uncovered = %v, want [db1.example.com]", got.Diff.Uncovered)
	}

	// --group narrows the inventory side: only the web group, so nothing is
	// uncovered any more.
	out, code = runCLI(t, bin, "targets", "--config", cfg, "--against", inv, "--group", "web")
	if code != 0 {
		t.Fatalf("--group exited %d:\n%s", code, out)
	}
	if !strings.Contains(out, "every inventory host is covered") {
		t.Errorf("with --group web everything should be covered:\n%s", out)
	}
}

// A coverage gap is information for a human, not a build failure: the command
// must stay exit 0 (the M31 rule for diagnostic commands).
func TestTargetsNeverGates(t *testing.T) {
	bin := buildCLI(t)
	dir := t.TempDir()
	cfg := writeTargetsConfig(t, dir)
	inv := filepath.Join(dir, "hosts.ini")
	if err := os.WriteFile(inv, []byte("[db]\nnothing-monitored\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, code := runCLI(t, bin, "targets", "--config", cfg, "--against", inv)
	if code != 0 {
		t.Fatalf("targets must exit 0 even with an uncovered host, got %d:\n%s", code, out)
	}
	if !strings.Contains(out, "not monitored") {
		t.Errorf("the gap should still be reported:\n%s", out)
	}
}

func TestTargetsSystemicErrors(t *testing.T) {
	bin := buildCLI(t)
	dir := t.TempDir()
	cfg := writeTargetsConfig(t, dir)

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"missing config", []string{"targets", "--config", filepath.Join(dir, "nope.yml")}, "no such file"},
		{"bad output", []string{"targets", "--config", cfg, "--output", "yaml"}, "unknown --output"},
		{"group without against", []string{"targets", "--config", cfg, "--group", "web"}, "--against"},
		{"missing inventory", []string{"targets", "--config", cfg, "--against", filepath.Join(dir, "nope.ini")}, "inventory"},
		{"unknown module", []string{"targets", "--config", cfg, "--module", "kafka"}, "no targets for module"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, code := runCLI(t, bin, tt.args...)
			if code != 1 {
				t.Fatalf("want exit 1 (systemic), got %d:\n%s", code, out)
			}
			if !strings.Contains(out, tt.want) {
				t.Errorf("error should mention %q:\n%s", tt.want, out)
			}
		})
	}
}
