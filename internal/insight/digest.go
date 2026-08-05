package insight

import (
	"fmt"
	"sort"
	"strings"
)

// Digest is what changed between two points in the history (CF-128), in the
// shape a human reads first: not a diff of every row, but the four movements
// that matter — what broke, what recovered, what got worse, what started
// oscillating.
type Digest struct {
	// New are targets that are a problem now and were not before.
	New []Change
	// Resolved are targets that were a problem and are now OK.
	Resolved []Change
	// Degraded are targets that were already a problem and got worse.
	Degraded []Change
	// Improved are targets still not OK but better than before.
	Improved []Change
	// Flapping are targets that started oscillating within the window.
	Flapping []string
	// Runs is how many records the comparison spanned.
	Runs int
}

// Change is one target's movement between the two ends of the window.
type Change struct {
	Check  string
	Target string
	From   string
	To     string
}

func (c Change) label() string { return c.Check + " " + c.Target }

// Empty reports whether nothing moved. A digest of nothing should say so in one
// line rather than print four empty headings.
func (d Digest) Empty() bool {
	return len(d.New)+len(d.Resolved)+len(d.Degraded)+len(d.Improved)+len(d.Flapping) == 0
}

// severity orders statuses for "better/worse" comparisons. Unknown statuses
// (an older schema, a future one) sort as OK so they never fabricate a
// regression out of a value this build does not understand.
var severity = map[string]int{"OK": 0, "WARN": 1, "ERROR": 2, "BAD": 3}

// Compare produces the digest between the first and last record of the window.
// Targets present at only one end are reported as appeared/disappeared through
// New/Resolved, because for an operator "it is broken now and wasn't in the
// report" and "it is new and broken" are the same news.
func Compare(series []StatusSeries, flapChanges int) Digest {
	d := Digest{}
	unstable := UnstableKeys(series, flapChanges)
	for _, s := range series {
		if len(s.Points) == 0 {
			continue
		}
		if d.Runs < len(s.Points) {
			d.Runs = len(s.Points)
		}
		from := s.Points[0].Status
		to := s.Points[len(s.Points)-1].Status
		c := Change{Check: s.Check, Target: s.Target, From: from, To: to}
		switch {
		case from == to:
			// unchanged; only instability below can still make it interesting
		case severity[to] == 0:
			d.Resolved = append(d.Resolved, c)
		case severity[from] == 0:
			d.New = append(d.New, c)
		case severity[to] > severity[from]:
			d.Degraded = append(d.Degraded, c)
		default:
			d.Improved = append(d.Improved, c)
		}
		if unstable[s.Key()] {
			d.Flapping = append(d.Flapping, c.label())
		}
	}
	sortChanges(d.New)
	sortChanges(d.Resolved)
	sortChanges(d.Degraded)
	sortChanges(d.Improved)
	sort.Strings(d.Flapping)
	return d
}

func sortChanges(cs []Change) {
	sort.Slice(cs, func(i, j int) bool { return cs[i].label() < cs[j].label() })
}

// Narrate renders the digest as prose an operator can forward as-is — to a
// chat sink, an issue, or the top of an incident doc.
func Narrate(d Digest) string {
	if d.Empty() {
		return fmt.Sprintf("Nothing changed across the last %d run(s).\n", d.Runs)
	}
	var parts []string
	if n := len(d.New); n > 0 {
		parts = append(parts, fmt.Sprintf("%d new problem(s)", n))
	}
	if n := len(d.Degraded); n > 0 {
		parts = append(parts, fmt.Sprintf("%d worse", n))
	}
	if n := len(d.Improved); n > 0 {
		parts = append(parts, fmt.Sprintf("%d improved", n))
	}
	if n := len(d.Resolved); n > 0 {
		parts = append(parts, fmt.Sprintf("%d resolved", n))
	}
	if n := len(d.Flapping); n > 0 {
		parts = append(parts, fmt.Sprintf("%d flapping", n))
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Across the last %d run(s): %s.\n", d.Runs, strings.Join(parts, ", "))
	writeSection(&b, "New", d.New)
	writeSection(&b, "Worse", d.Degraded)
	writeSection(&b, "Improved", d.Improved)
	writeSection(&b, "Resolved", d.Resolved)
	if len(d.Flapping) > 0 {
		fmt.Fprintf(&b, "\nFlapping:\n")
		for _, f := range d.Flapping {
			fmt.Fprintf(&b, "  - %s\n", f)
		}
	}
	return b.String()
}

func writeSection(b *strings.Builder, title string, cs []Change) {
	if len(cs) == 0 {
		return
	}
	fmt.Fprintf(b, "\n%s:\n", title)
	for _, c := range cs {
		fmt.Fprintf(b, "  - %s: %s → %s\n", c.label(), c.From, c.To)
	}
}
