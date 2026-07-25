package output

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// SelfMetrics renders per-module Prometheus metrics about checkfleet itself:
// how many findings and measurement errors each module produced in the last run.
// Appended after the target metrics on the exporter's /metrics endpoint so
// operators can alert on a module that keeps failing. Run duration and last-run
// timestamp are already emitted by Prometheus(), so they are not repeated here.
func SelfMetrics(res engine.Result) string {
	// Per-module counts.
	type counts struct{ findings, errors int }
	byModule := map[string]*counts{}
	var order []string
	for _, f := range res.Findings {
		c, ok := byModule[f.Check]
		if !ok {
			c = &counts{}
			byModule[f.Check] = c
			order = append(order, f.Check)
		}
		c.findings++
		if f.Status == engine.ERROR {
			c.errors++
		}
	}
	sort.Strings(order)

	var b strings.Builder
	b.WriteString("# HELP checkfleet_module_findings Number of findings produced by a module in the last run.\n")
	b.WriteString("# TYPE checkfleet_module_findings gauge\n")
	for _, m := range order {
		fmt.Fprintf(&b, "checkfleet_module_findings{module=%q} %d\n", m, byModule[m].findings)
	}

	b.WriteString("# HELP checkfleet_module_errors Number of ERROR findings (measurement failures) for a module.\n")
	b.WriteString("# TYPE checkfleet_module_errors gauge\n")
	for _, m := range order {
		fmt.Fprintf(&b, "checkfleet_module_errors{module=%q} %d\n", m, byModule[m].errors)
	}
	return b.String()
}
