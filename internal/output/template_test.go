package output

import (
	"strings"
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

func TestRenderTemplate(t *testing.T) {
	res := engine.Result{Findings: []engine.Finding{
		{Check: "certs", Target: "api:443", Status: engine.BAD, Message: "expired"},
		{Check: "http", Target: "x", Status: engine.OK, Message: "ok"},
	}}
	tmpl := `{"worst":"{{.Worst}}","total":{{.Total}},"bad":{{.BAD}},"first":"{{(index .Findings 0).Check}}"}`
	out, err := RenderTemplate(res, "prod", tmpl)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"worst":"BAD","total":2,"bad":1,"first":"certs"}`
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestRenderTemplateRangeAndError(t *testing.T) {
	res := engine.Result{Findings: []engine.Finding{
		{Check: "http", Target: "x", Status: engine.WARN, Message: "slow"},
	}}
	out, err := RenderTemplate(res, "t", `{{range .Findings}}{{.Status}}:{{.Target}};{{end}}`)
	if err != nil || !strings.Contains(out, "WARN:x;") {
		t.Fatalf("range template: %q err=%v", out, err)
	}
	// A bad template surfaces an error rather than silent output.
	if _, err := RenderTemplate(res, "t", `{{.Nope}}`); err == nil {
		t.Error("unknown field should error (missingkey=error)")
	}
}
