package insight

import (
	"net"
	"sort"
	"strings"

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
		{"host", hostOf},
		{"subnet", subnetOf},
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

// hostOf extracts the host from a target. Targets are spelled differently by
// module — "db-01:5432", "https://a.example/health", "pg-integration" — so this
// handles the shapes the modules actually produce and gives up otherwise.
func hostOf(f engine.Finding) string {
	t := f.Target
	// A module may qualify a sub-finding as "target/aspect" (postgres does).
	if i := strings.IndexByte(t, '/'); i > 0 && !strings.Contains(t, "://") {
		t = t[:i]
	}
	if strings.Contains(t, "://") {
		// URL: take the authority, drop credentials, port and path.
		rest := t[strings.Index(t, "://")+3:]
		if i := strings.IndexAny(rest, "/?#"); i >= 0 {
			rest = rest[:i]
		}
		if i := strings.LastIndexByte(rest, '@'); i >= 0 {
			rest = rest[i+1:]
		}
		t = rest
	}
	if h, _, err := net.SplitHostPort(t); err == nil {
		return h
	}
	if t == "" || strings.ContainsAny(t, " \t") {
		return ""
	}
	return t
}

// subnetOf returns the /24 of an IPv4 target, or "" when the host is not a
// literal IPv4 address. Names are not resolved: an insight must not do DNS.
func subnetOf(f engine.Finding) string {
	ip := net.ParseIP(hostOf(f))
	if ip == nil {
		return ""
	}
	v4 := ip.To4()
	if v4 == nil {
		return ""
	}
	return v4.Mask(net.CIDRMask(24, 32)).String() + "/24"
}
