package insight

import (
	"sort"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// Cluster is a group of problems that share one dimension (CF-123). Thirty red
// rows that are all one host, or all one module, are one incident; reading them
// as thirty is how a real blast radius gets missed in the scroll.
type Cluster struct {
	// Dimension is what the members share: "host", "module" or "subnet".
	Dimension string
	// Value is the shared value — the hostname, module name or /24.
	Value string
	// Findings are the problems in the group, in the order they arrived.
	Findings []engine.Finding
}

// Size is how many problems the cluster covers.
func (c Cluster) Size() int { return len(c.Findings) }

// MinClusterSize is the smallest group worth calling a pattern. Two findings
// sharing a host is a coincidence you can read directly off the table.
const MinClusterSize = 3

// Correlate groups the non-OK findings by the dimensions they share and returns
// the clusters worth showing, largest first.
//
// Each finding lands in at most one cluster: the most specific dimension wins
// (host, then subnet, then module). Reporting the same outage three times under
// three headings would be the same wall of rows with extra steps.
func Correlate(findings []engine.Finding) []Cluster {
	var problems []engine.Finding
	for _, f := range findings {
		if f.Status != engine.OK {
			problems = append(problems, f)
		}
	}
	if len(problems) < MinClusterSize {
		return nil
	}

	claimed := make([]bool, len(problems))
	var out []Cluster
	// Most specific first, so a single broken host is reported as a host outage
	// rather than dissolved into "some postgres checks failed".
	for _, dim := range []struct {
		name string
		of   func(engine.Finding) string
	}{
		{"host", func(f engine.Finding) string { return engine.HostOf(f.Target) }},
		{"subnet", func(f engine.Finding) string { return engine.SubnetOf(f.Target) }},
		{"module", func(f engine.Finding) string { return f.Check }},
	} {
		groups := map[string][]int{}
		var order []string
		for i, f := range problems {
			if claimed[i] {
				continue
			}
			v := dim.of(f)
			if v == "" {
				continue
			}
			if _, seen := groups[v]; !seen {
				order = append(order, v)
			}
			groups[v] = append(groups[v], i)
		}
		for _, v := range order {
			idx := groups[v]
			if len(idx) < MinClusterSize {
				continue
			}
			c := Cluster{Dimension: dim.name, Value: v}
			for _, i := range idx {
				claimed[i] = true
				c.Findings = append(c.Findings, problems[i])
			}
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Size() > out[j].Size() })
	return out
}
