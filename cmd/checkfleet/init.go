package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/Allan-Nava/checkfleet/internal/scaffold"
)

// runInit scaffolds a starter checkfleet.yml for the chosen modules.
func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	configPath := fs.String("config", "checkfleet.yml", "path of the config file to create")
	modules := fs.String("modules", "", "comma-separated modules to include (default: certs,http)")
	force := fs.Bool("force", false, "overwrite the file if it already exists")
	list := fs.Bool("list", false, "list the modules init can scaffold and exit")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: checkfleet init [--config checkfleet.yml] [--modules certs,http,...] [--force] [--list]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *list {
		fmt.Println("Modules init can scaffold:")
		fmt.Println("  " + strings.Join(scaffold.Supported(), ", "))
		return nil
	}

	var mods []string
	if strings.TrimSpace(*modules) != "" {
		for _, m := range strings.Split(*modules, ",") {
			if m = strings.TrimSpace(m); m != "" {
				mods = append(mods, m)
			}
		}
	}

	content, err := scaffold.Config(mods)
	if err != nil {
		return err
	}

	if _, err := os.Stat(*configPath); err == nil && !*force {
		return fmt.Errorf("%s already exists (use --force to overwrite)", *configPath)
	}
	if err := os.WriteFile(*configPath, []byte(content), 0o644); err != nil {
		return err
	}

	fmt.Printf("checkfleet: wrote %s\n", *configPath)
	fmt.Printf("Next: edit the placeholders, then run `checkfleet check all --config %s`\n", *configPath)
	return nil
}
