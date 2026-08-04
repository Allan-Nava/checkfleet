package output

import (
	"strings"
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// hintedResult has one annotated problem, one bare problem and one OK finding,
// so a renderer that leaks the hint onto the wrong row is caught.
func hintedResult() engine.Result {
	return engine.Result{Findings: []engine.Finding{
		{Check: "certs", Target: "api.example:443", Status: engine.BAD, Message: "expires in 2 days",
			Runbook: "https://wiki.example/tls", Remediation: "Renew and reload"},
		{Check: "redis", Target: "cache:6379", Status: engine.WARN, Message: "evictions rising"},
		{Check: "http", Target: "https://ok.example", Status: engine.OK, Message: "200 in 12ms"},
	}}
}

func TestTextShowsHintUnderTheFinding(t *testing.T) {
	out := Text(hintedResult())
	if !strings.Contains(out, "↳ Renew and reload — https://wiki.example/tls") {
		t.Errorf("text output missing the hint line:\n%s", out)
	}
	if strings.Count(out, "↳") != 1 {
		t.Errorf("want exactly one hint line, got %d:\n%s", strings.Count(out, "↳"), out)
	}
}

func TestMarkdownLinksTheRunbookInNeedsAttention(t *testing.T) {
	out := Markdown(hintedResult(), "test")
	if !strings.Contains(out, "Renew and reload — [runbook](https://wiki.example/tls)") {
		t.Errorf("markdown missing the hint cell:\n%s", out)
	}
	// The full table below stays a plain inventory: the hint appears once.
	if got := strings.Count(out, "[runbook]"); got != 1 {
		t.Errorf("want the runbook link once (Needs attention only), got %d", got)
	}
	// The documented four-column shape must not change.
	if !strings.Contains(out, "| Status | Check | Target | Detail |") {
		t.Errorf("markdown table header changed — that is a compatibility surface")
	}
}

func TestHTMLRendersHintAndEscapes(t *testing.T) {
	out := HTML(hintedResult(), "test")
	if !strings.Contains(out, `<a href="https://wiki.example/tls">runbook</a>`) {
		t.Errorf("html missing the runbook link:\n%s", out)
	}
	if !strings.Contains(out, "Renew and reload") {
		t.Errorf("html missing the remediation note")
	}
}

func TestHTMLDoesNotLinkNonHTTPRunbook(t *testing.T) {
	res := engine.Result{Findings: []engine.Finding{
		{Check: "certs", Target: "a", Status: engine.BAD, Message: "m",
			Runbook: "javascript:alert(1)"},
	}}
	out := HTML(res, "test")
	if strings.Contains(out, "<a href=\"javascript:") {
		t.Errorf("a non-http runbook must not become a link:\n%s", out)
	}
	if !strings.Contains(out, "javascript:alert(1)") {
		t.Errorf("the value should still be shown as inert text")
	}
}

func TestJSONOmitsHintsWhenAbsent(t *testing.T) {
	out, err := JSON(hintedResult())
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(out, `"runbook"`); got != 1 {
		t.Errorf(`want "runbook" on the one annotated finding, got %d occurrences`, got)
	}
	if strings.Contains(out, `"runbook": ""`) {
		t.Errorf("empty hints must be omitted, not emitted as empty strings")
	}
}

// clusteredResult has one dead host (three modules) plus an unrelated problem.
func clusteredResult() engine.Result {
	f := func(check, target string, s engine.Status) engine.Finding {
		return engine.Finding{Check: check, Target: target, Status: s, Message: "detail"}
	}
	return engine.Result{Findings: []engine.Finding{
		f("postgres", "db-01:5432", engine.BAD),
		f("redis", "db-01:6379", engine.BAD),
		f("tcp", "db-01:22", engine.BAD),
		f("http", "https://web-01/", engine.WARN),
		f("certs", "a.example:443", engine.OK),
	}}
}

func TestMarkdownCollapsesCorrelatedFailures(t *testing.T) {
	out := Markdown(clusteredResult(), "test")
	if !strings.Contains(out, "## 🔗 Correlated failures") {
		t.Fatalf("missing the correlated section:\n%s", out)
	}
	if !strings.Contains(out, "<details>") {
		t.Error("clusters must be collapsed, not a second wall of rows")
	}
	if !strings.Contains(out, "<b>3 failures</b> share the same host: <code>db-01</code>") {
		t.Errorf("cluster summary wrong:\n%s", out)
	}
}

func TestMarkdownOmitsTheSectionWithoutAPattern(t *testing.T) {
	out := Markdown(hintedResult(), "test") // two unrelated problems
	if strings.Contains(out, "Correlated failures") {
		t.Error("a handful of unrelated problems must not grow a correlation section")
	}
}

func TestMarkdownCarriesTheFleetScore(t *testing.T) {
	clean := Markdown(engine.Result{Findings: []engine.Finding{
		{Check: "http", Target: "a", Status: engine.OK, Message: "m"},
	}}, "test")
	if !strings.Contains(clean, "**Fleet health: 100.0/100**") {
		t.Errorf("an all-green run should score 100:\n%s", clean)
	}
	broken := Markdown(clusteredResult(), "test")
	if strings.Contains(broken, "100.0/100") {
		t.Error("a run with three BAD findings must not score 100")
	}
}
