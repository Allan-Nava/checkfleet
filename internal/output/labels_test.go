package output

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

func labelledResult() engine.Result {
	res := resultFrom([]engine.Finding{
		{Check: "http", Target: "x/health", Status: engine.BAD, Message: "500"},
	})
	res.Labels = map[string]string{"env": "prod", "data-center": "eu-1"}
	return res
}

// Global labels ride on every Prometheus series, and an invalid label name is
// sanitized (CF-119).
func TestPrometheusLabels(t *testing.T) {
	out := Prometheus(labelledResult())
	for _, want := range []string{
		`checkfleet_finding_status{check="http",target="x/health",data_center="eu-1",env="prod"} 2`,
		`checkfleet_findings_total{status="OK",data_center="eu-1",env="prod"} 0`,
		`checkfleet_worst_status{data_center="eu-1",env="prod"} 2`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Prometheus output missing labelled line:\n%s\n---\n%s", want, out)
		}
	}
}

// With no labels the series stay label-clean (back-compat).
func TestPrometheusNoLabels(t *testing.T) {
	out := Prometheus(resultFrom([]engine.Finding{{Check: "http", Target: "t", Status: engine.OK, Message: "ok"}}))
	if strings.Contains(out, "checkfleet_worst_status{") {
		t.Errorf("worst_status should have no braces without labels:\n%s", out)
	}
}

// JSON carries the labels as a top-level field.
func TestJSONLabels(t *testing.T) {
	s, err := JSON(labelledResult())
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Labels map[string]string `json:"labels"`
	}
	if err := json.Unmarshal([]byte(s), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if got.Labels["env"] != "prod" || got.Labels["data-center"] != "eu-1" {
		t.Fatalf("labels missing/wrong in JSON: %v", got.Labels)
	}
}

// OTLP carries the labels as resource attributes.
func TestOTLPLabels(t *testing.T) {
	s, err := OTLP(labelledResult())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, `"env"`) || !strings.Contains(s, `"prod"`) {
		t.Fatalf("OTLP resource attributes should include the labels:\n%s", s)
	}
}

// A webhook template can read .Labels.
func TestTemplateLabels(t *testing.T) {
	out, err := RenderTemplate(labelledResult(), "all", `{{ .Labels.env }}`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "prod" {
		t.Fatalf("template .Labels.env = %q, want prod", out)
	}
}
