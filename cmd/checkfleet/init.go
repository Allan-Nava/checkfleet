package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/Allan-Nava/checkfleet/internal/inventory"
	"github.com/Allan-Nava/checkfleet/internal/scaffold"
)

// runInit scaffolds a starter checkfleet.yml for the chosen modules.
func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	configPath := fs.String("config", "checkfleet.yml", "path of the config file to create")
	modules := fs.String("modules", "", "comma-separated modules to include (default: certs,http)")
	recipe := fs.String("recipe", "", "starter stack to scaffold: web, db, edge or media")
	fromInventory := fs.String("from-inventory", "", "Ansible inventory file or directory: generate the targets from its hosts")
	group := fs.String("group", "", "with --from-inventory: only use hosts in this group")
	force := fs.Bool("force", false, "overwrite the file if it already exists")
	list := fs.Bool("list", false, "list the modules and recipes init can scaffold, and exit")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: checkfleet init [--config checkfleet.yml] [--modules certs,http,...] [--recipe web|db|edge|media] [--from-inventory hosts.ini [--group web]] [--force] [--list]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *list {
		fmt.Println("Modules init can scaffold:")
		fmt.Println("  " + strings.Join(scaffold.Supported(), ", "))
		fmt.Println("\nRecipes (--recipe):")
		for _, name := range scaffold.Recipes() {
			r, _ := scaffold.RecipeByName(name)
			fmt.Printf("  %-6s %s\n     modules: %s\n", r.Name, r.Summary, strings.Join(r.Modules, ", "))
		}
		fmt.Println("\nModules that can be generated from an inventory (--from-inventory):")
		fmt.Println("  " + strings.Join(scaffold.InventoryModules(), ", "))
		return nil
	}

	if *recipe != "" && *fromInventory != "" {
		return fmt.Errorf("--recipe and --from-inventory pick the targets in different ways; use one")
	}
	if *group != "" && *fromInventory == "" {
		return fmt.Errorf("--group only applies with --from-inventory")
	}
	if *recipe != "" && strings.TrimSpace(*modules) != "" {
		return fmt.Errorf("--recipe already selects the modules; drop --modules")
	}

	var mods []string
	if strings.TrimSpace(*modules) != "" {
		for _, m := range strings.Split(*modules, ",") {
			if m = strings.TrimSpace(m); m != "" {
				mods = append(mods, m)
			}
		}
	}

	var content string
	var err error
	switch {
	case *recipe != "":
		content, err = scaffold.ConfigForRecipe(*recipe)
	case *fromInventory != "":
		content, err = configFromInventory(*fromInventory, *group, mods)
	default:
		content, err = scaffold.Config(mods)
	}
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

// configFromInventory loads the inventory, narrows it to a group if asked, and
// renders the config from its hosts.
func configFromInventory(path, group string, modules []string) (string, error) {
	hosts, err := inventory.LoadPath(path)
	if err != nil {
		return "", fmt.Errorf("reading inventory %s: %w", path, err)
	}
	if group != "" {
		var kept []inventory.Host
		for _, h := range hosts {
			if h.Group == group {
				kept = append(kept, h)
			}
		}
		if len(kept) == 0 {
			return "", fmt.Errorf("no hosts in group %q in %s", group, path)
		}
		hosts = kept
	}
	if len(hosts) == 0 {
		return "", fmt.Errorf("no hosts found in %s", path)
	}
	return scaffold.FromInventory(hosts, modules)
}
