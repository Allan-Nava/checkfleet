package main

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// startTCP starts a throwaway TCP listener that accepts and immediately closes
// connections, so the tcp check sees a reachable target. Returns host:port.
func startTCP(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	return ln.Addr().String()
}

func writeConfig(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return p
}

// RunChecks against a reachable tcp target must succeed, report the module and a
// finding, and the exports must render the cached result.
func TestRunChecks_TCP_OK(t *testing.T) {
	addr := startTCP(t)
	cfg := writeConfig(t, "checkfleet.yml",
		"timeout_seconds: 5\nchecks:\n  tcp:\n    targets:\n      - address: \""+addr+"\"\n")

	app := NewApp("test")
	rep := app.RunChecks(cfg, "")

	if rep.Err != "" {
		t.Fatalf("unexpected error: %s", rep.Err)
	}
	if len(rep.Findings) == 0 {
		t.Fatal("expected at least one finding")
	}
	if !contains(rep.Modules, "tcp") {
		t.Fatalf("modules = %v, want it to include tcp", rep.Modules)
	}
	if rep.Worst != "OK" {
		t.Fatalf("worst = %q, want OK for a reachable target", rep.Worst)
	}
	if rep.OK < 1 {
		t.Fatalf("ok count = %d, want >= 1", rep.OK)
	}

	// Exports render the cached run.
	if md := app.ExportMarkdown(); !strings.Contains(md, "tcp") {
		t.Fatalf("markdown export missing the tcp finding:\n%s", md)
	}
	js, err := app.ExportJSON()
	if err != nil {
		t.Fatalf("json export: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(js), &payload); err != nil {
		t.Fatalf("json export is not valid JSON: %v", err)
	}
}

func TestRunChecks_Errors(t *testing.T) {
	app := NewApp("test")

	if rep := app.RunChecks("", ""); rep.Err == "" {
		t.Fatal("empty config path should report an error")
	}
	if rep := app.RunChecks("/no/such/checkfleet.yml", ""); rep.Err == "" {
		t.Fatal("missing config file should report an error")
	}
	// A valid file with no modules configured is an error too.
	empty := writeConfig(t, "checkfleet.yml", "timeout_seconds: 5\nchecks: {}\n")
	if rep := app.RunChecks(empty, ""); rep.Err == "" {
		t.Fatal("config with no modules should report an error")
	}
}

func TestListStacks(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"checkfleet.yml", "checkfleet.prod.yml", "checkfleet.edge.yaml", "unrelated.yml"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("checks: {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	app := NewApp("test")
	got := app.ListStacks(filepath.Join(dir, "checkfleet.yml"))
	// Sorted, base excluded, unrelated.yml ignored.
	if len(got) != 2 || got[0] != "edge" || got[1] != "prod" {
		t.Fatalf("ListStacks = %v, want [edge prod]", got)
	}
	if app.ListStacks("") != nil {
		t.Fatal("ListStacks(\"\") should be nil")
	}
}

func TestDefaultConfigPath(t *testing.T) {
	app := NewApp("test")

	dir := t.TempDir()
	t.Chdir(dir)
	if got := app.DefaultConfigPath(); got != "" {
		t.Fatalf("no checkfleet.yml present, got %q", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "checkfleet.yml"), []byte("checks: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := app.DefaultConfigPath(); got == "" {
		t.Fatal("checkfleet.yml present but DefaultConfigPath returned empty")
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
