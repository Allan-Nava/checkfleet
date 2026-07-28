package moduledoc_test

// moduledoc is the single source behind `checkfleet explain`, the SARIF rule
// descriptions and the desktop's module chips (CF-158). It is a hand-written map,
// which makes it exactly the kind of thing that goes stale the moment a module is
// added — the same failure that left the intro of docs/modules.md claiming 18
// modules when there were 29. These tests are the guard.

import (
	"strings"
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/moduledoc"
	"github.com/Allan-Nava/checkfleet/internal/registry"
)

// registryModules is every module the binary knows about, configured or not.
func registryModules() []string { return registry.All(&engine.Config{}) }

func TestEveryRegistryModuleIsDocumented(t *testing.T) {
	for _, name := range registryModules() {
		doc, ok := moduledoc.Doc(name)
		if !ok {
			t.Errorf("module %q has no entry in moduledoc.Docs — `checkfleet explain %s` would say nothing, and its SARIF rule would ship undescribed", name, name)
			continue
		}
		if strings.TrimSpace(doc) == "" {
			t.Errorf("module %q has an empty description", name)
		}
	}
}

// The reverse direction: a doc for a module that no longer exists is a promise
// the binary cannot keep, and it would show up in the desktop as a chip that
// leads nowhere.
func TestNoDocForAnUnknownModule(t *testing.T) {
	known := make(map[string]bool)
	for _, name := range registryModules() {
		known[name] = true
	}
	for name := range moduledoc.Docs {
		if !known[name] {
			t.Errorf("moduledoc documents %q, which is not a module in the registry", name)
		}
	}
}

// A description is what someone reads to decide whether this is the module for
// their symptom. Only the length is asserted: it catches a placeholder entry
// added in a hurry, and any cleverer heuristic about "quality" just fails on
// perfectly good prose.
func TestDescriptionsAreNotPlaceholders(t *testing.T) {
	for _, name := range registryModules() {
		doc, _ := moduledoc.Doc(name)
		if len(doc) < 40 {
			t.Errorf("module %q: description is too short to be useful (%d chars): %q", name, len(doc), doc)
		}
		if strings.EqualFold(strings.TrimSpace(doc), name) {
			t.Errorf("module %q: the description just restates the name", name)
		}
	}
}

// No credentials in text that is printed by `explain` and embedded in reports.
func TestDescriptionsCarryNoSecrets(t *testing.T) {
	for name, doc := range moduledoc.Docs {
		lower := strings.ToLower(doc)
		for _, bad := range []string{"password=", "://user:", "secret=", "token="} {
			if strings.Contains(lower, bad) {
				t.Errorf("module %q description looks like it contains a credential (%q): %q", name, bad, doc)
			}
		}
	}
}

func TestDocUnknownModule(t *testing.T) {
	if _, ok := moduledoc.Doc("carrier-pigeon"); ok {
		t.Error("an unknown module must report not-found rather than an empty string")
	}
}
