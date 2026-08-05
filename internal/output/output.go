// Package output renders a run's Result as terminal text, markdown (in the
// ops-report style: problems first, full table after) or JSON.
package output

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/insight"
)

var statusIcon = map[engine.Status]string{
	engine.OK:    "🟢",
	engine.WARN:  "🟡",
	engine.BAD:   "🔴",
	engine.ERROR: "⛔",
}

// ANSI colours for the terminal renderer (used only when colour is enabled).
const ansiReset = "\x1b[0m"

var statusColor = map[engine.Status]string{
	engine.OK:    "\x1b[32m", // green
	engine.WARN:  "\x1b[33m", // yellow
	engine.BAD:   "\x1b[31m", // red
	engine.ERROR: "\x1b[35m", // magenta
}

// hintLine renders a finding's operator hints (CF-124) as one plain line, or ""
// when it carries none. Only findings above OK ever carry them.
func hintLine(f engine.Finding) string {
	switch {
	case f.Remediation != "" && f.Runbook != "":
		return f.Remediation + " — " + f.Runbook
	case f.Remediation != "":
		return f.Remediation
	default:
		return f.Runbook
	}
}

// hintCell renders the hints for a markdown table cell: the note as text and the
// runbook as a link, on a second line inside the same cell so the four-column
// shape of the table (a documented surface) is unchanged.
func hintCell(f engine.Finding) string {
	var parts []string
	if f.Remediation != "" {
		parts = append(parts, f.Remediation)
	}
	if f.Runbook != "" {
		parts = append(parts, "[runbook]("+f.Runbook+")")
	}
	if len(parts) == 0 {
		return ""
	}
	return "<br>↳ " + strings.Join(parts, " — ")
}

func summaryLine(res engine.Result) string {
	s := engine.Summarize(res.Findings)
	return fmt.Sprintf("%d checks: %d OK, %d WARN, %d BAD, %d ERROR (in %s)",
		len(res.Findings), s[engine.OK], s[engine.WARN], s[engine.BAD], s[engine.ERROR],
		res.Duration.Round(time.Millisecond))
}

// Text renders for the terminal: worst findings first (Result is pre-sorted).
func Text(res engine.Result) string { return textRender(res, false, nil) }

// TextColor is Text with ANSI colour on the status column. The caller decides
// when colour is appropriate (a TTY, no NO_COLOR, not redirected to a file).
func TextColor(res engine.Result) string { return textRender(res, true, nil) }

// TextWith is Text plus the M30 analyses under the findings (CF-173). rep may
// be nil, which is exactly Text.
func TextWith(res engine.Result, color bool, rep *insight.Report) string {
	return textRender(res, color, rep)
}

func textRender(res engine.Result, color bool, rep *insight.Report) string {
	var b strings.Builder
	for _, f := range res.Findings {
		status := fmt.Sprintf("%-5s", f.Status)
		if color {
			// ANSI codes are zero-width, so column alignment is preserved.
			status = statusColor[f.Status] + status + ansiReset
		}
		fmt.Fprintf(&b, "%s %s %-8s %-45s %s\n", statusIcon[f.Status], status, f.Check, f.Target, f.Message)
		if h := hintLine(f); h != "" {
			fmt.Fprintf(&b, "        ↳ %s\n", h)
		}
	}
	fmt.Fprintf(&b, "\n%s\n", summaryLine(res))
	if rep != nil && !rep.Empty() {
		fmt.Fprintf(&b, "\n%s", insight.Text(*rep, insight.TextOptions{}))
	}
	return b.String()
}

// Markdown renders an ops-style report: summary, problems, full table.
func Markdown(res engine.Result, title string) string { return markdownWith(res, title, nil) }

// MarkdownWith is Markdown plus the M30 analyses (CF-173). rep may be nil.
func MarkdownWith(res engine.Result, title string, rep *insight.Report) string {
	return markdownWith(res, title, rep)
}

func markdownWith(res engine.Result, title string, rep *insight.Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# checkfleet — %s\n\n", title)
	fmt.Fprintf(&b, "Generated: %s\n\n", res.Started.Format(time.RFC3339))
	fmt.Fprintf(&b, "```\n%s\n```\n\n", summaryLine(res))
	// The fleet index (CF-127). Status-only here: the renderer has no history,
	// so the instability surcharge is `insight --score`'s job, not this one.
	fmt.Fprintf(&b, "**Fleet health: %.1f/100**\n\n", insight.FleetScore(res.Findings, nil).Value)

	var problems []engine.Finding
	for _, f := range res.Findings {
		if f.Status != engine.OK {
			problems = append(problems, f)
		}
	}
	fmt.Fprintf(&b, "## ⚠ Needs attention\n\n")
	if len(problems) == 0 {
		fmt.Fprintf(&b, "Nothing — all green. ✅\n\n")
	} else {
		fmt.Fprintf(&b, "| Status | Check | Target | Detail |\n|---|---|---|---|\n")
		for _, f := range problems {
			// The hints ride in the Detail cell of this section only: it is the
			// actionable one, and the full table below stays a plain inventory.
			fmt.Fprintf(&b, "| %s %s | %s | `%s` | %s%s |\n",
				statusIcon[f.Status], f.Status, f.Check, f.Target, f.Message, hintCell(f))
		}
		fmt.Fprintf(&b, "\n")
	}

	writeClusters(&b, res.Findings)

	fmt.Fprintf(&b, "## All results\n\n| Status | Check | Target | Detail |\n|---|---|---|---|\n")
	for _, f := range res.Findings {
		fmt.Fprintf(&b, "| %s %s | %s | `%s` | %s |\n", statusIcon[f.Status], f.Status, f.Check, f.Target, f.Message)
	}
	if rep != nil && !rep.Empty() {
		fmt.Fprintf(&b, "\n## 📈 Insight\n\n```\n%s```\n", insight.Text(*rep, insight.TextOptions{}))
	}
	return b.String()
}

// writeClusters renders the correlated-failure groups (CF-123), collapsed so
// they add a line to the report rather than a second wall of rows. Nothing is
// written when no group reaches the threshold — the plain table is already the
// right shape for a handful of unrelated problems.
func writeClusters(b *strings.Builder, findings []engine.Finding) {
	clusters := insight.Correlate(findings)
	if len(clusters) == 0 {
		return
	}
	fmt.Fprintf(b, "## 🔗 Correlated failures\n\n")
	for _, c := range clusters {
		fmt.Fprintf(b, "<details><summary><b>%d failures</b> share the same %s: <code>%s</code></summary>\n\n",
			c.Size(), c.Dimension, c.Value)
		fmt.Fprintf(b, "| Status | Check | Target | Detail |\n|---|---|---|---|\n")
		for _, f := range c.Findings {
			fmt.Fprintf(b, "| %s %s | %s | `%s` | %s |\n", statusIcon[f.Status], f.Status, f.Check, f.Target, f.Message)
		}
		fmt.Fprintf(b, "\n</details>\n\n")
	}
}

// JSONSchemaVersion is the version of the JSON output document, emitted as the
// top-level "schema" field (CF-153). The JSON output is a parsed interface —
// pipelines gate on "worst", dashboards read "findings" — so it needs a way to
// signal a breaking change instead of silently meaning something else.
//
// Bump it only when an existing field changes meaning or disappears; adding a
// field is backward compatible and does not warrant a bump.
const JSONSchemaVersion = 1

// JSON renders the machine-readable result.
func JSON(res engine.Result) (string, error) { return JSONWith(res, nil) }

// JSONWith is JSON plus an "insight" block carrying the M30 analyses (CF-173).
// The field is additive and omitempty, so a run without history produces
// byte-identical output to before and the schema version does not move.
func JSONWith(res engine.Result, rep *insight.Report) (string, error) {
	out, err := json.MarshalIndent(struct {
		Schema int `json:"schema"`
		engine.Result
		Summary map[engine.Status]int `json:"summary"`
		Worst   engine.Status         `json:"worst"`
		Insight *insight.Report       `json:"insight,omitempty"`
	}{JSONSchemaVersion, res, engine.Summarize(res.Findings), engine.Worst(res.Findings), rep}, "", "  ")
	return string(out), err
}
