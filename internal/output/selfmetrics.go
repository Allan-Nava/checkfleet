package output

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// SelfMetrics renders Prometheus metrics about checkfleet itself (not the
// targets): how long the last run took, when it ran, and how many findings /
// measurement errors each module produced. Appended after the target metrics on
// the exporter's /metrics endpoint so operators can alert on the checker itself.
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
	b.WriteString("# HELP checkfleet_run_duration_seconds Wall-clock duration of the last check run.\n")
	b.WriteString("# TYPE checkfleet_run_duration_seconds gauge\n")
	fmt.Fprintf(&b, "checkfleet_run_duration_seconds %g\n", res.Duration.Seconds())

	b.WriteString("# HELP checkfleet_last_run_timestamp_seconds Unix time the last run started.\n")
	b.WriteString("# TYPE checkfleet_last_run_timestamp_seconds gauge\n")
	fmt.Fprintf(&b, "checkfleet_last_run_timestamp_seconds %d\n", res.Started.Unix())

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
