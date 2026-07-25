// Package output renders a run's Result as terminal text, markdown (in the
// ops-report style: problems first, full table after) or JSON.
package output

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
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

func summaryLine(res engine.Result) string {
	s := engine.Summarize(res.Findings)
	return fmt.Sprintf("%d checks: %d OK, %d WARN, %d BAD, %d ERROR (in %s)",
		len(res.Findings), s[engine.OK], s[engine.WARN], s[engine.BAD], s[engine.ERROR],
		res.Duration.Round(time.Millisecond))
}

// Text renders for the terminal: worst findings first (Result is pre-sorted).
func Text(res engine.Result) string { return textRender(res, false) }

// TextColor is Text with ANSI colour on the status column. The caller decides
// when colour is appropriate (a TTY, no NO_COLOR, not redirected to a file).
func TextColor(res engine.Result) string { return textRender(res, true) }

func textRender(res engine.Result, color bool) string {
	var b strings.Builder
	for _, f := range res.Findings {
		status := fmt.Sprintf("%-5s", f.Status)
		if color {
			// ANSI codes are zero-width, so column alignment is preserved.
			status = statusColor[f.Status] + status + ansiReset
		}
		fmt.Fprintf(&b, "%s %s %-8s %-45s %s\n", statusIcon[f.Status], status, f.Check, f.Target, f.Message)
	}
	fmt.Fprintf(&b, "\n%s\n", summaryLine(res))
	return b.String()
}

// Markdown renders an ops-style report: summary, problems, full table.
func Markdown(res engine.Result, title string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# checkfleet — %s\n\n", title)
	fmt.Fprintf(&b, "Generated: %s\n\n", res.Started.Format(time.RFC3339))
	fmt.Fprintf(&b, "```\n%s\n```\n\n", summaryLine(res))

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
			fmt.Fprintf(&b, "| %s %s | %s | `%s` | %s |\n", statusIcon[f.Status], f.Status, f.Check, f.Target, f.Message)
		}
		fmt.Fprintf(&b, "\n")
	}

	fmt.Fprintf(&b, "## All results\n\n| Status | Check | Target | Detail |\n|---|---|---|---|\n")
	for _, f := range res.Findings {
		fmt.Fprintf(&b, "| %s %s | %s | `%s` | %s |\n", statusIcon[f.Status], f.Status, f.Check, f.Target, f.Message)
	}
	return b.String()
}

// JSON renders the machine-readable result.
func JSON(res engine.Result) (string, error) {
	out, err := json.MarshalIndent(struct {
		engine.Result
		Summary map[engine.Status]int `json:"summary"`
		Worst   engine.Status         `json:"worst"`
	}{res, engine.Summarize(res.Findings), engine.Worst(res.Findings)}, "", "  ")
	return string(out), err
}
