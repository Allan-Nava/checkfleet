package output

import (
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// HTML renders a run as a self-contained static HTML report: a summary with the
// worst status and per-status counts, a "Needs attention" section for the
// non-OK findings, and a full table. Styles are inlined (no external
// resources), themed to match the docs site. Safe to paste into an incident
// doc or serve as an artifact.
func HTML(res engine.Result, title string) string {
	sum := engine.Summarize(res.Findings)
	worst := string(engine.Worst(res.Findings))

	var problems []engine.Finding
	for _, f := range res.Findings {
		if f.Status != engine.OK {
			problems = append(problems, f)
		}
	}

	var b strings.Builder
	b.WriteString("<!doctype html>\n<html lang=\"en\"><head><meta charset=\"utf-8\">")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">")
	fmt.Fprintf(&b, "<title>checkfleet — %s</title>\n", html.EscapeString(title))
	b.WriteString("<style>" + htmlCSS + "</style></head><body>\n")

	fmt.Fprintf(&b, "<header><h1>check<b>fleet</b> — %s</h1>", html.EscapeString(title))
	fmt.Fprintf(&b, "<span class=\"gen\">Generated %s</span></header>\n", res.Started.Format(time.RFC3339))

	// summary
	b.WriteString("<section class=\"summary\">")
	fmt.Fprintf(&b, "<div class=\"worst s-%s\"><b>%s</b><span>worst status</span></div>", worst, worst)
	b.WriteString("<div class=\"tiles\">")
	for _, s := range []engine.Status{engine.OK, engine.WARN, engine.BAD, engine.ERROR} {
		fmt.Fprintf(&b, "<div class=\"tile s-%s\"><b>%d</b><span>%s</span></div>", s, sum[s], s)
	}
	b.WriteString("</div></section>\n")
	fmt.Fprintf(&b, "<p class=\"sub\">%s</p>\n", html.EscapeString(summaryLine(res)))

	// needs attention
	b.WriteString("<h2>⚠ Needs attention</h2>\n")
	if len(problems) == 0 {
		b.WriteString("<p class=\"ok-note\">Nothing — all green. ✅</p>\n")
	} else {
		writeTable(&b, problems)
	}

	// all results
	b.WriteString("<h2>All results</h2>\n")
	writeTable(&b, res.Findings)

	b.WriteString("</body></html>\n")
	return b.String()
}

func writeTable(b *strings.Builder, findings []engine.Finding) {
	b.WriteString("<table><thead><tr><th>Status</th><th>Check</th><th>Target</th><th>Message</th></tr></thead><tbody>\n")
	for _, f := range findings {
		fmt.Fprintf(b, "<tr><td><span class=\"badge s-%s\">%s</span></td><td class=\"mono\">%s</td><td class=\"mono\">%s</td><td>%s</td></tr>\n",
			f.Status, f.Status,
			html.EscapeString(f.Check), html.EscapeString(f.Target), html.EscapeString(f.Message))
	}
	b.WriteString("</tbody></table>\n")
}

const htmlCSS = `
:root{--bg:#0b1120;--surface:#111c34;--border:rgba(148,163,184,.14);--text:#dbe4f0;--muted:#93a4bd;--heading:#f1f5f9;--brand:#10b981;--ok:#34d399;--warn:#fbbf24;--bad:#f87171;--err:#c084fc}
*{box-sizing:border-box}
body{margin:0;padding:28px;background:var(--bg);color:var(--text);font:15px/1.6 -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Helvetica,Arial,sans-serif}
h1{font-size:22px;margin:0;color:var(--heading)} h1 b{color:var(--brand)}
h2{font-size:17px;color:var(--heading);margin:28px 0 10px}
header{display:flex;align-items:baseline;gap:14px;flex-wrap:wrap;border-bottom:1px solid var(--border);padding-bottom:14px}
.gen{color:var(--muted);font-size:13px}
.sub{color:var(--muted);font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:13px}
.summary{display:flex;gap:12px;flex-wrap:wrap;margin-top:18px}
.worst,.tile{border:1px solid var(--border);border-radius:12px;background:var(--surface);padding:12px 18px;display:flex;flex-direction:column;justify-content:center}
.worst b{font-size:20px}.worst span,.tile span{font-size:11px;color:var(--muted);text-transform:uppercase;letter-spacing:.06em}
.tiles{display:flex;gap:10px;flex-wrap:wrap}.tile b{font-size:22px}
.s-OK b,.badge.s-OK{color:var(--ok)}.s-WARN b,.badge.s-WARN{color:var(--warn)}
.s-BAD b,.badge.s-BAD{color:var(--bad)}.s-ERROR b,.badge.s-ERROR{color:var(--err)}
table{width:100%;border-collapse:collapse;font-size:14px;margin:6px 0 4px}
th,td{text-align:left;padding:8px 12px;border-bottom:1px solid var(--border);vertical-align:top}
th{color:var(--muted);font-size:11px;text-transform:uppercase;letter-spacing:.05em}
.mono{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:13px}
.badge{font-weight:700;font-size:12px}
.ok-note{color:var(--ok)}
@media (prefers-color-scheme:light){:root{--bg:#fff;--surface:#f4f7fb;--border:rgba(15,23,42,.1);--text:#33415c;--muted:#64748b;--heading:#0f172a;--brand:#059669;--ok:#059669;--warn:#b45309;--bad:#dc2626;--err:#7c3aed}}
`
