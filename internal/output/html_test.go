package output

import (
	"strings"
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

func TestHTMLReport(t *testing.T) {
	html := HTML(fixtureResult(), "all")

	if !strings.HasPrefix(strings.TrimSpace(html), "<!doctype html>") {
		t.Fatalf("HTML report should start with a doctype:\n%.80s", html)
	}
	// Self-contained: styles inlined, no external references.
	if !strings.Contains(html, "<style>") {
		t.Error("report should inline its CSS")
	}
	if strings.Contains(html, "http://") || strings.Contains(html, "https://cdn") {
		t.Error("report must be self-contained (no external resources)")
	}
	// Summary carries the worst status and the counts.
	if !strings.Contains(html, "BAD") {
		t.Error("summary should show the worst status BAD")
	}
	for _, want := range []string{"1 OK", "1 WARN", "1 BAD", "0 ERROR"} {
		if !strings.Contains(html, want) {
			t.Errorf("summary missing %q", want)
		}
	}
	// A problem and an OK both render.
	if !strings.Contains(html, "bad.example") || !strings.Contains(html, "ok.example") {
		t.Error("findings not rendered")
	}
}

func TestHTMLEscapesContent(t *testing.T) {
	res := engine.Result{Findings: []engine.Finding{
		{Check: "http", Target: "x", Status: engine.BAD, Message: `<script>alert(1)</script>`},
	}}
	html := HTML(res, "all")
	if strings.Contains(html, "<script>alert(1)</script>") {
		t.Error("finding message must be HTML-escaped")
	}
	if !strings.Contains(html, "&lt;script&gt;") {
		t.Errorf("expected escaped message in output")
	}
}
