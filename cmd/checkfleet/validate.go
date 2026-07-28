// The `validate` command: inspect a config without running any check.

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// runValidate checks the config without running any check; exit 1 if invalid.
//
//	checkfleet validate --config checkfleet.yml [--stack …]
func runValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	configPath := fs.String("config", "checkfleet.yml", "YAML config file")
	stack := fs.String("stack", "", "comma-separated stack profiles overlaid in order (last wins): checkfleet.<stack>.yml onto the base")
	if err := fs.Parse(args); err != nil {
		return err
	}
	// A config that fails to load is still worth inspecting: the reason is often
	// a misspelled key or an unset variable, which Inspect can name even when the
	// typed load could not complete.
	cfg, loadErr := loadConfig(*configPath, *stack)
	problems := engine.Inspect(*configPath, cfg)

	if loadErr != nil {
		fmt.Fprintf(os.Stderr, "%s: cannot be loaded: %v\n", *configPath, loadErr)
		for _, p := range problems {
			fmt.Fprintln(os.Stderr, "  -", p)
		}
		os.Exit(1)
	}
	if !engine.Blocking(problems) {
		fmt.Printf("checkfleet: %s is valid ✅\n", *configPath)
		// Advisory notes are about this machine, not the config, so they are
		// printed after the all-clear rather than turning it into a failure.
		for _, p := range problems {
			fmt.Printf("  note: %s\n", p)
		}
		return nil
	}
	fmt.Fprintf(os.Stderr, "%s: %d problem(s):\n", *configPath, len(problems))
	for _, p := range problems {
		prefix := "  -"
		if p.Advisory {
			prefix = "  note:"
		}
		fmt.Fprintln(os.Stderr, prefix, p)
	}
	os.Exit(1)
	return nil
}
