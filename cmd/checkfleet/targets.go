package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/coverage"
	"github.com/Allan-Nava/checkfleet/internal/inventory"
)

// runTargets lists every configured target across modules, optionally diffed
// against an Ansible inventory to answer "is every host actually monitored?".
//
// It never runs a check and never gates: this is a diagnostic, so it exits 0
// unless something systemic fails (unreadable config or inventory). A coverage
// gap is a finding for a human, not a build failure.
//
//	checkfleet targets --config checkfleet.yml [--output text|json]
//	    [--against hosts.ini] [--group web] [--module certs]
func runTargets(args []string) error {
	fs := flag.NewFlagSet("targets", flag.ExitOnError)
	configPath := fs.String("config", "checkfleet.yml", "YAML config file")
	stack := fs.String("stack", "", "comma-separated stack profiles overlaid in order (last wins)")
	format := fs.String("output", "text", "output format: text or json")
	against := fs.String("against", "", "Ansible inventory file or directory to diff the coverage against")
	group := fs.String("group", "", "with --against: only consider inventory hosts in this group")
	module := fs.String("module", "", "only list targets of this module")
	noDiscover := fs.Bool("no-discover", false, "skip discovery sources (consul_service, dns_srv, ansible_inventory) and list only the targets written in the config")
	discoverTimeout := fs.Duration("discover-timeout", 15*time.Second, "budget for resolving all discovery sources")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *format != "text" && *format != "json" {
		return fmt.Errorf("unknown --output %q (use text or json)", *format)
	}
	if *group != "" && *against == "" {
		return fmt.Errorf("--group only applies with --against INVENTORY")
	}

	cfg, err := loadConfig(*configPath, *stack)
	if err != nil {
		return err
	}
	// Coverage is exactly the question a misspelled module gives a wrong answer
	// to: the target simply isn't there, and nothing says why.
	warnUnknownKeys(os.Stderr, *configPath, *stack)
	targets := coverage.Targets(cfg)

	// Discovery runs before anything else so this command answers what a run
	// would actually cover (CF-179). Skippable because it talks to the network:
	// a catalog or nameserver that is down must not stop an operator from
	// reading the static half of the picture.
	if !*noDiscover {
		ctx, cancel := context.WithTimeout(context.Background(), *discoverTimeout)
		defer cancel()
		found, derrs := coverage.Discovered(ctx, cfg)
		targets = append(targets, found...)
		// Reported on stderr, not as an exit code: this is a diagnostic, and a
		// half-resolved fleet is still worth printing. Silence here would be
		// the failure mode the whole feature exists to avoid.
		for _, m := range sortedKeys(derrs) {
			fmt.Fprintf(os.Stderr, "warning: %s: discovery failed: %v\n", m, derrs[m])
		}
	}

	if *module != "" {
		var kept []coverage.Target
		for _, t := range targets {
			if t.Module == *module {
				kept = append(kept, t)
			}
		}
		if len(kept) == 0 {
			return fmt.Errorf("no targets for module %q in %s", *module, *configPath)
		}
		targets = kept
	}

	var diff *coverage.Diff
	if *against != "" {
		hosts, err := inventory.LoadPath(*against)
		if err != nil {
			return fmt.Errorf("reading inventory %s: %w", *against, err)
		}
		if *group != "" {
			var kept []inventory.Host
			for _, h := range hosts {
				if h.Group == *group {
					kept = append(kept, h)
				}
			}
			if len(kept) == 0 {
				return fmt.Errorf("no hosts in group %q in %s", *group, *against)
			}
			hosts = kept
		}
		d := coverage.DiffInventory(targets, hosts)
		diff = &d
	}

	if *format == "json" {
		out := struct {
			Targets []coverage.Target `json:"targets"`
			Diff    *coverage.Diff    `json:"coverage,omitempty"`
		}{targets, diff}
		b, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	}

	fmt.Print(formatTargets(targets, diff, *against))
	return nil
}

// formatTargets renders the text view: targets grouped by module, then the
// inventory diff when one was requested.
func formatTargets(targets []coverage.Target, diff *coverage.Diff, inventoryPath string) string {
	var b strings.Builder

	byModule := map[string][]coverage.Target{}
	var order []string
	for _, t := range targets {
		if _, seen := byModule[t.Module]; !seen {
			order = append(order, t.Module) // registry order, as enumerated
		}
		byModule[t.Module] = append(byModule[t.Module], t)
	}

	fmt.Fprintf(&b, "%d target(s) across %d module(s)\n", len(targets), len(order))
	for _, m := range order {
		fmt.Fprintf(&b, "\n%s (%d)\n", m, len(byModule[m]))
		for _, t := range byModule[m] {
			// The hosts are shown only when they add something to the label, so
			// the common case stays a single readable column.
			hosts := strings.Join(t.Hosts, ", ")
			// The source is shown only for discovered targets: it is the answer
			// to "where did this host come from?", which is a question a
			// hand-written target never raises.
			src := ""
			if t.Source != "" {
				src = "  [" + t.Source + "]"
			}
			if hosts != "" && hosts != t.Name {
				fmt.Fprintf(&b, "  %-52s → %s%s\n", t.Name, hosts, src)
			} else {
				fmt.Fprintf(&b, "  %s%s\n", t.Name, src)
			}
		}
	}

	if diff == nil {
		return b.String()
	}

	total := len(diff.Covered) + len(diff.Uncovered)
	fmt.Fprintf(&b, "\ncoverage vs %s: %d/%d inventory host(s) covered\n",
		inventoryPath, len(diff.Covered), total)

	if len(diff.Uncovered) > 0 {
		fmt.Fprintf(&b, "\nnot monitored (%d)\n", len(diff.Uncovered))
		for _, h := range sortedCopy(diff.Uncovered) {
			fmt.Fprintf(&b, "  %s\n", h)
		}
	}
	if len(diff.Extra) > 0 {
		// Not an error: external dependencies legitimately aren't in your
		// inventory. Worth showing because a typo in a target looks the same.
		fmt.Fprintf(&b, "\ntargeted but not in the inventory (%d)\n", len(diff.Extra))
		for _, h := range sortedCopy(diff.Extra) {
			fmt.Fprintf(&b, "  %s\n", h)
		}
	}
	if len(diff.Uncovered) == 0 && total > 0 {
		fmt.Fprint(&b, "\nevery inventory host is covered ✅\n")
	}
	return b.String()
}

// sortedKeys keeps the discovery warnings in a stable order, so a CI log diffs
// cleanly between runs.
func sortedKeys(m map[string]error) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
