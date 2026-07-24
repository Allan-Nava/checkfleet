package output

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// severity maps a status to the numeric value exposed as a gauge.
var severity = map[engine.Status]int{engine.OK: 0, engine.WARN: 1, engine.BAD: 2, engine.ERROR: 3}

// Prometheus renders a run in the Prometheus text exposition format.
//
// One gauge per (check,target) carries the finding severity
// (0=OK,1=WARN,2=BAD,3=ERROR); if a (check,target) pair appears more than once
// the worst severity wins, so the series stays unique. Roll-up gauges expose
// the per-status counts, the overall worst, the run duration and its time.
func Prometheus(res engine.Result) string {
	var b strings.Builder

	// Collapse to the worst severity per (check,target) to keep series unique.
	type key struct{ check, target string }
	worst := map[key]int{}
	order := []key{}
	for _, f := range res.Findings {
		k := key{f.Check, f.Target}
		s := severity[f.Status]
		if cur, ok := worst[k]; ok {
			if s > cur {
				worst[k] = s
			}
			continue
		}
		worst[k] = s
		order = append(order, k)
	}
	sort.Slice(order, func(i, j int) bool {
		if order[i].check != order[j].check {
			return order[i].check < order[j].check
		}
		return order[i].target < order[j].target
	})

	b.WriteString("# HELP checkfleet_finding_status Finding severity (0=OK,1=WARN,2=BAD,3=ERROR).\n")
	b.WriteString("# TYPE checkfleet_finding_status gauge\n")
	for _, k := range order {
		fmt.Fprintf(&b, "checkfleet_finding_status{check=\"%s\",target=\"%s\"} %d\n", esc(k.check), esc(k.target), worst[k])
	}

	sum := engine.Summarize(res.Findings)
	b.WriteString("# HELP checkfleet_findings_total Number of findings by status.\n")
	b.WriteString("# TYPE checkfleet_findings_total gauge\n")
	for _, st := range []engine.Status{engine.OK, engine.WARN, engine.BAD, engine.ERROR} {
		fmt.Fprintf(&b, "checkfleet_findings_total{status=%q} %d\n", st, sum[st])
	}

	b.WriteString("# HELP checkfleet_worst_status Worst severity across all findings.\n")
	b.WriteString("# TYPE checkfleet_worst_status gauge\n")
	fmt.Fprintf(&b, "checkfleet_worst_status %d\n", severity[engine.Worst(res.Findings)])

	b.WriteString("# HELP checkfleet_run_duration_seconds Duration of the last run.\n")
	b.WriteString("# TYPE checkfleet_run_duration_seconds gauge\n")
	fmt.Fprintf(&b, "checkfleet_run_duration_seconds %g\n", res.Duration.Seconds())

	b.WriteString("# HELP checkfleet_last_run_timestamp_seconds Unix time of the last run.\n")
	b.WriteString("# TYPE checkfleet_last_run_timestamp_seconds gauge\n")
	fmt.Fprintf(&b, "checkfleet_last_run_timestamp_seconds %d\n", res.Started.Unix())

	return b.String()
}

// esc escapes a Prometheus label value (backslash, quote, newline).
func esc(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}
