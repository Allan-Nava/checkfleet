package output

import (
	"fmt"
	"sort"
	"strings"
	"time"

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

// SampleAges exposes how long ago each module last produced a sample (CF-178).
//
// With per-module cadences a stale metric is normal, so "the exporter stopped
// updating" is no longer a usable alarm and the age per module is what you
// alert on instead: `checkfleet_sample_age_seconds{check="certs"} > 7200`.
// Without it an hourly module looks frozen and there is no way to tell that
// apart from a module that is genuinely stuck.
func SampleAges(ages map[string]time.Duration) string {
	if len(ages) == 0 {
		return ""
	}
	names := make([]string, 0, len(ages))
	for n := range ages {
		names = append(names, n)
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString("# HELP checkfleet_sample_age_seconds Seconds since this module last produced findings.\n")
	b.WriteString("# TYPE checkfleet_sample_age_seconds gauge\n")
	for _, n := range names {
		fmt.Fprintf(&b, "checkfleet_sample_age_seconds{check=%q} %.0f\n", n, ages[n].Seconds())
	}
	return b.String()
}
