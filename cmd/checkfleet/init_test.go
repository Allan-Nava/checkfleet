package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunInitWritesConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "checkfleet.yml")
	if err := runInit([]string{"--config", path, "--modules", "certs,http"}); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading generated file: %v", err)
	}
	if !strings.Contains(string(b), "certs:") || !strings.Contains(string(b), "http:") {
		t.Errorf("generated file missing modules:\n%s", b)
	}
}

func TestRunInitRefusesOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "checkfleet.yml")
	if err := runInit([]string{"--config", path}); err != nil {
		t.Fatal(err)
	}
	if err := runInit([]string{"--config", path}); err == nil {
		t.Error("second init without --force should fail")
	}
	if err := runInit([]string{"--config", path, "--force"}); err != nil {
		t.Errorf("init --force should overwrite, got %v", err)
	}
}

func TestRunInitUnknownModule(t *testing.T) {
	path := filepath.Join(t.TempDir(), "checkfleet.yml")
	if err := runInit([]string{"--config", path, "--modules", "nope"}); err == nil {
		t.Error("unknown module should error")
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("no file should be written when a module is unknown")
	}
}
