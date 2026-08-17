package engine

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// DependsRule declares that some findings are consequences of another (CF-174).
//
// A dead host produces one finding per module that touches it, and `alert`
// opens them all. CF-123 taught the report to *group* them; this decides what
// leaves the gate and the notifications.
//
// Matching mirrors the other rules: globs on check and target, empty meaning
// all. The parent is named by OnCheck plus either an explicit OnTarget glob or
// SameHost, which pairs a finding with the parent on the same host — reusing
// HostOf, so "which host is this" means one thing across the codebase.
type DependsRule struct {
	Check  string `yaml:"check"`  // glob on the dependent check; "" = all
	Target string `yaml:"target"` // glob on the dependent target; "" = all
	// OnCheck is the module the dependents rely on (e.g. tcp, or a ping check).
	OnCheck string `yaml:"on_check"`
	// OnTarget is a glob on the parent's target. Ignored when SameHost is set.
	OnTarget string `yaml:"on_target"`
	// SameHost pairs each dependent with the OnCheck finding on its own host,
	// which is the common case: "everything on db-01 depends on db-01 answering".
	//
	// It compares HostOf(target), so it only works when both findings spell
	// their target as a host or host:port. A module configured with a friendly
	// `name:` reports that name, which shares no host with an address as far as
	// any comparison goes — the rule then matches nothing, silently. Use an
	// explicit OnTarget for named targets.
	SameHost bool `yaml:"same_host"`
}

// Suppressed is the status a consequence is downgraded to. WARN, not dropped:
// a finding that disappears is indistinguishable from a check that never ran,
// and "the fleet went quiet" is the worst possible way to learn about an
// outage. The row stays on screen, marked, and out of the gate.
const Suppressed = WARN

// ApplyDependencies downgrades findings whose declared parent is itself failing,
// annotating them with what suppressed them.
//
// Only BAD/ERROR parents suppress: a parent that is merely WARN has not
// explained its children away.
func ApplyDependencies(findings []Finding, rules []DependsRule) []Finding {
	if len(rules) == 0 {
		return findings
	}
	// Index the failing parents by check, so each dependent is a lookup rather
	// than a scan of the whole run.
	failing := map[string][]Finding{}
	for _, f := range findings {
		if f.Status == BAD || f.Status == ERROR {
			failing[f.Check] = append(failing[f.Check], f)
		}
	}

	out := findings[:0:0] // new slice, keep the input untouched
	for _, f := range findings {
		if f.Status == OK || f.Status == Suppressed {
			out = append(out, f)
			continue
		}
		if parent, ok := parentOf(f, rules, failing); ok {
			f.Status = Suppressed
			f.SuppressedBy = parent
			f.Message += " [suppressed by " + parent + "]"
		}
		out = append(out, f)
	}
	return out
}

// parentOf returns the label of the failing parent that explains f, if any.
func parentOf(f Finding, rules []DependsRule, failing map[string][]Finding) (string, bool) {
	for _, r := range rules {
		if r.OnCheck == "" || !globMatch(r.Check, f.Check) || !globMatch(r.Target, f.Target) {
			continue
		}
		for _, p := range failing[r.OnCheck] {
			if p.Check == f.Check && p.Target == f.Target {
				continue // a finding never suppresses itself
			}
			switch {
			case r.SameHost:
				if HostOf(p.Target) != "" && HostOf(p.Target) == HostOf(f.Target) {
					return p.Check + " " + p.Target, true
				}
			case r.OnTarget != "":
				if globMatch(r.OnTarget, p.Target) {
					return p.Check + " " + p.Target, true
				}
			}
		}
	}
	return "", false
}

// globMatch treats an empty pattern as "everything".
func globMatch(pattern, s string) bool {
	if pattern == "" {
		return true
	}
	ok, _ := path.Match(pattern, s)
	return ok
}

// ValidateDependencies reports rules that are malformed or that form a cycle
// (CF-174). A cycle would leave a run's outcome depending on the order the
// findings happened to arrive in, so it is refused at validate time rather than
// resolved arbitrarily at run time.
func ValidateDependencies(rules []DependsRule) []string {
	var problems []string
	for i, r := range rules {
		if r.OnCheck == "" {
			problems = append(problems, fmt.Sprintf("depends_on[%d]: on_check is required", i))
			continue
		}
		if !r.SameHost && r.OnTarget == "" {
			problems = append(problems, fmt.Sprintf(
				"depends_on[%d]: set same_host: true, or an on_target glob — otherwise the rule names no parent", i))
		}
		if r.Check != "" && r.Check == r.OnCheck && r.SameHost {
			problems = append(problems, fmt.Sprintf(
				"depends_on[%d]: %s depends on itself on the same host", i, r.Check))
		}
	}
	problems = append(problems, cycleProblems(rules)...)
	sort.Strings(problems)
	return problems
}

// cycleProblems walks the check-level dependency graph and names any cycle.
func cycleProblems(rules []DependsRule) []string {
	edges := map[string][]string{}
	for _, r := range rules {
		if r.Check == "" || r.OnCheck == "" {
			continue // a catch-all rule has no single node to hang an edge on
		}
		edges[r.Check] = append(edges[r.Check], r.OnCheck)
	}
	var problems []string
	seen := map[string]bool{}
	var stack []string
	inStack := map[string]bool{}

	var walk func(node string)
	walk = func(node string) {
		if inStack[node] {
			// Report from the first occurrence, so the message shows the loop.
			at := 0
			for i, n := range stack {
				if n == node {
					at = i
					break
				}
			}
			problems = append(problems, "depends_on: cycle "+strings.Join(append(stack[at:], node), " → "))
			return
		}
		if seen[node] {
			return
		}
		seen[node] = true
		inStack[node] = true
		stack = append(stack, node)
		for _, next := range edges[node] {
			walk(next)
		}
		stack = stack[:len(stack)-1]
		inStack[node] = false
	}
	starts := make([]string, 0, len(edges))
	for n := range edges {
		starts = append(starts, n)
	}
	sort.Strings(starts) // deterministic message for the same config
	for _, n := range starts {
		walk(n)
	}
	return problems
}
