package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/registry"
)

// A module count written into prose is a claim that goes stale in silence. It
// already has: the README still said "29 modules ship today" with 30 in the
// registry, and before that the intro of docs/modules.md sat at 18 with 29
// shipped. Both are the first thing a reader sees, so this pins every such
// claim to the registry instead of to whoever last remembered.
func TestProseModuleCountsMatchTheRegistry(t *testing.T) {
	want := len(registry.Modules(&engine.Config{}))

	root := filepath.Join("..", "..")
	re := regexp.MustCompile(`(\d+) modules? ship today`)
	for _, rel := range []string{"README.md", "docs/index.md", "docs/modules.md"} {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range re.FindAllStringSubmatch(string(b), -1) {
			got, err := strconv.Atoi(m[1])
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Errorf("%s claims %d modules, the registry has %d — update the prose (%q)",
					rel, got, want, m[0])
			}
		}
	}
}
