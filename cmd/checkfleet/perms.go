package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/moduledoc"
	"github.com/Allan-Nava/checkfleet/internal/registry"
)

// runPerms prints the least privilege each module needs on the system it
// watches (CF-182), so the operator can send *that* to the DBA instead of a
// vague request.
//
// With --config it covers only the modules actually configured, which is the
// useful default: a fleet that runs six modules should not hand over grants for
// twenty-nine. Without one it covers every module, or the one named.
//
// Exit-code semantics: this is not a check. 0 on success, non-zero only on a
// systemic failure (unreadable config, unknown module).
//
// It prints grant statements, never credentials: every example that sets a
// password carries a placeholder, enforced by a test in internal/moduledoc.
func runPerms(args []string) error {
	// The module is positional and may come before the flags, which is the
	// natural way to type it (`perms redis --output json`). Go's flag package
	// stops parsing at the first non-flag argument, so extracting it first is
	// what makes that form work — the same shape `check <module> --flags` uses.
	var named string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		named, args = args[0], args[1:]
	}

	fs := flag.NewFlagSet("perms", flag.ContinueOnError)
	configPath := fs.String("config", "", "restrict the output to the modules configured in this file")
	stack := fs.String("stack", "", "comma-separated stack profiles overlaid in order (with --config)")
	output := fs.String("output", "text", "text|markdown|json")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if named == "" {
		named = fs.Arg(0) // also accept it after the flags
	}
	names, err := permModules(named, *configPath, *stack)
	if err != nil {
		return err
	}
	switch *output {
	case "json":
		return permsJSON(names)
	case "markdown":
		fmt.Print(permsMarkdown(names))
		return nil
	case "text":
		fmt.Print(permsText(names))
		return nil
	default:
		return fmt.Errorf("unknown format %q (use text, markdown or json)", *output)
	}
}

// permModules resolves which modules to report on.
func permModules(named, configPath, stack string) ([]string, error) {
	if named != "" {
		if _, ok := moduledoc.Doc(named); !ok {
			return nil, fmt.Errorf("unknown module %q (run: checkfleet explain)", named)
		}
		return []string{named}, nil
	}
	if configPath == "" {
		all := registry.All(&engine.Config{})
		sort.Strings(all)
		return all, nil
	}
	cfg, err := loadConfig(configPath, stack)
	if err != nil {
		return nil, err
	}
	names := registry.Names(cfg)
	if len(names) == 0 {
		return nil, fmt.Errorf("no module configured in %s", configPath)
	}
	sort.Strings(names)
	return names, nil
}

func permsText(names []string) string {
	var b strings.Builder
	var free, auth []string
	for _, n := range names {
		if p, ok := moduledoc.Perms(n); ok && p.Unauthenticated {
			free = append(free, n)
			continue
		}
		auth = append(auth, n)
	}

	if len(free) > 0 {
		fmt.Fprintf(&b, "No credential needed (%d): %s\n\n", len(free), strings.Join(free, ", "))
	}
	for _, n := range auth {
		p, ok := moduledoc.Perms(n)
		if !ok {
			fmt.Fprintf(&b, "%s\n  (no permissions entry — please report this)\n\n", n)
			continue
		}
		fmt.Fprintf(&b, "%s\n  %s\n", n, wrapAt(p.Summary, 76, "  "))
		for _, s := range p.Statements {
			for _, line := range strings.Split(s, "\n") {
				fmt.Fprintf(&b, "    %s\n", line)
			}
		}
		fmt.Fprintf(&b, "  Not needed: %s\n", wrapAt(p.NotNeeded, 76, "  "))
		if p.NeedsJudgement {
			fmt.Fprintf(&b, "  NOTE: depends on how the target is deployed — guidance, not a recipe.\n")
		}
		b.WriteString("\n")
	}
	if len(auth) == 0 {
		b.WriteString("Nothing to grant: every configured module reads what the target already exposes.\n")
	}
	return b.String()
}

func permsMarkdown(names []string) string {
	var b strings.Builder
	b.WriteString("# checkfleet — required permissions\n\n")
	for _, n := range names {
		p, ok := moduledoc.Perms(n)
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "## %s\n\n%s\n\n", n, p.Summary)
		if len(p.Statements) > 0 {
			b.WriteString("```\n")
			for _, s := range p.Statements {
				b.WriteString(s + "\n")
			}
			b.WriteString("```\n\n")
		}
		fmt.Fprintf(&b, "**Not needed:** %s\n\n", p.NotNeeded)
	}
	return b.String()
}

// permEntry is the JSON shape, for a pipeline that generates its own runbook.
type permEntry struct {
	Module          string   `json:"module"`
	Summary         string   `json:"summary"`
	Statements      []string `json:"statements,omitempty"`
	NotNeeded       string   `json:"not_needed"`
	Unauthenticated bool     `json:"unauthenticated"`
	NeedsJudgement  bool     `json:"needs_judgement,omitempty"`
}

func permsJSON(names []string) error {
	out := make([]permEntry, 0, len(names))
	for _, n := range names {
		p, ok := moduledoc.Perms(n)
		if !ok {
			continue
		}
		out = append(out, permEntry{
			Module: n, Summary: p.Summary, Statements: p.Statements,
			NotNeeded: p.NotNeeded, Unauthenticated: p.Unauthenticated,
			NeedsJudgement: p.NeedsJudgement,
		})
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(map[string]any{"permissions": out})
}

// wrapAt soft-wraps prose so a terminal at 80 columns stays readable.
func wrapAt(s string, width int, indent string) string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return ""
	}
	var b strings.Builder
	line := 0
	for i, w := range words {
		if i > 0 && line+1+len(w) > width {
			b.WriteString("\n" + indent)
			line = 0
		} else if i > 0 {
			b.WriteString(" ")
			line++
		}
		b.WriteString(w)
		line += len(w)
	}
	return b.String()
}
