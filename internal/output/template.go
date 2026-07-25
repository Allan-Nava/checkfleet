package output

import (
	"strings"
	"text/template"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// TemplateData is what a webhook template is executed against. It exposes the
// summary rollup and the findings (worst first) with friendly field names.
type TemplateData struct {
	Title    string
	Worst    string
	Total    int
	OK       int
	WARN     int
	BAD      int
	ERROR    int
	Findings []engine.Finding
}

// RenderTemplate executes a Go text/template against the run, for shaping a
// custom webhook payload. `missingkey=error` surfaces typos in the template
// instead of silently emitting "<no value>".
func RenderTemplate(res engine.Result, title, tmplText string) (string, error) {
	t, err := template.New("webhook").Option("missingkey=error").Parse(tmplText)
	if err != nil {
		return "", err
	}
	sum := engine.Summarize(res.Findings)
	data := TemplateData{
		Title:    title,
		Worst:    string(engine.Worst(res.Findings)),
		Total:    len(res.Findings),
		OK:       sum[engine.OK],
		WARN:     sum[engine.WARN],
		BAD:      sum[engine.BAD],
		ERROR:    sum[engine.ERROR],
		Findings: res.Findings,
	}
	var b strings.Builder
	if err := t.Execute(&b, data); err != nil {
		return "", err
	}
	return b.String(), nil
}
