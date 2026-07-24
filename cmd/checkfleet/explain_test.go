package main

import (
	"strings"
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/registry"
)

// Every registered module must have an explain entry (keeps docs from drifting).
func TestModuleDocsCoverAllModules(t *testing.T) {
	for _, m := range registry.All(&engine.Config{}) {
		if strings.TrimSpace(moduleDocs[m]) == "" {
			t.Errorf("module %q has no moduleDocs entry", m)
		}
	}
}

func TestRunExplainUnknown(t *testing.T) {
	if err := runExplain([]string{"nope"}); err == nil {
		t.Error("unknown module should error")
	}
	if err := runExplain([]string{"certs"}); err != nil {
		t.Errorf("known module should not error: %v", err)
	}
	if err := runExplain(nil); err != nil {
		t.Errorf("listing modules should not error: %v", err)
	}
}

func TestCompletionScript(t *testing.T) {
	for _, sh := range []string{"bash", "zsh", "fish"} {
		s, err := completionScript(sh)
		if err != nil {
			t.Fatalf("%s: %v", sh, err)
		}
		if !strings.Contains(s, "checkfleet") || !strings.Contains(s, "certs") {
			t.Errorf("%s: script missing expected tokens", sh)
		}
	}
	if _, err := completionScript("powershell"); err == nil {
		t.Error("unsupported shell should error")
	}
}
