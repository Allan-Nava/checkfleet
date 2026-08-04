package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSectionsAreSplitPerModule(t *testing.T) {
	got, err := readSections("../../docs/modules.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < 25 {
		t.Fatalf("split found %d sections — the heading pattern probably stopped matching", len(got))
	}
	// s3 exists to catch a digit in the module id: a `[a-z]+` pattern silently
	// drops it, and the page for it would never be generated.
	if _, ok := got["s3"]; !ok {
		t.Error("s3 section not found — the id pattern must allow digits")
	}
	if !strings.Contains(got["certs"], "expiry") {
		t.Errorf("certs prose looks wrong: %.80q", got["certs"])
	}
	// The body must stop at the next heading, not swallow the rest of the file.
	if strings.Contains(got["certs"], "## `http`") {
		t.Error("a section swallowed the following one")
	}
}

func TestEveryIndexedModuleHasAPage(t *testing.T) {
	mods, err := readModules("../../docs/_data/modules.yml")
	if err != nil {
		t.Fatal(err)
	}
	if len(mods) == 0 {
		t.Fatal("no modules in the index")
	}
	for _, m := range mods {
		path := filepath.Join("../../docs/modules", m.Name+".md")
		body, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("missing page for %q: %v", m.Name, err)
			continue
		}
		s := string(body)
		if !strings.Contains(s, "permalink: /modules/"+m.Name) {
			t.Errorf("%s: wrong or missing permalink", m.Name)
		}
		if !strings.Contains(s, "checkfleet check "+m.Name) {
			t.Errorf("%s: page does not show how to run the module", m.Name)
		}
		if !strings.Contains(s, genNote) {
			t.Errorf("%s: missing the generated-file marker", m.Name)
		}
	}
}

func TestGeneratorIsDeterministic(t *testing.T) {
	mods, err := readModules("../../docs/_data/modules.yml")
	if err != nil {
		t.Fatal(err)
	}
	prose, err := readSections("../../docs/modules.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range mods {
		first := page(m, prose[m.Name])
		for i := 0; i < 3; i++ {
			if page(m, prose[m.Name]) != first {
				t.Fatalf("page(%s) is not deterministic", m.Name)
			}
		}
	}
}

// TestPagesCarryNoSecrets — these are published to a public site.
func TestPagesCarryNoSecrets(t *testing.T) {
	entries, err := os.ReadDir("../../docs/modules")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		body, err := os.ReadFile(filepath.Join("../../docs/modules", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		low := strings.ToLower(string(body))
		for _, pat := range []string{"-----begin", "bearer ey"} {
			if strings.Contains(low, pat) {
				t.Errorf("%s contains %q", e.Name(), pat)
			}
		}
	}
}
