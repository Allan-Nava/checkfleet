package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/moduledoc"
	"github.com/Allan-Nava/checkfleet/internal/registry"
)

// runExplain prints what a module checks and its thresholds, or lists modules.
// Descriptions live in internal/moduledoc (shared with the desktop app).
//
//	checkfleet explain [module]
func runExplain(args []string) error {
	all := registry.All(&engine.Config{})
	if len(args) == 0 {
		fmt.Println("modules (checkfleet explain <module>):")
		sorted := append([]string(nil), all...)
		sort.Strings(sorted)
		for _, m := range sorted {
			fmt.Printf("  %-9s %s\n", m, firstSentence(moduledoc.Docs[m]))
		}
		return nil
	}
	m := args[0]
	doc, ok := moduledoc.Doc(m)
	if !ok {
		return fmt.Errorf("unknown module %q (run: checkfleet explain)", m)
	}
	fmt.Printf("%s — %s\n", m, doc)
	return nil
}

func firstSentence(s string) string {
	if i := strings.IndexByte(s, '.'); i >= 0 {
		return s[:i+1]
	}
	return s
}

// moduleNames returns every known module name (for completion/help).
func moduleNames() []string {
	names := registry.All(&engine.Config{})
	sort.Strings(names)
	return names
}
