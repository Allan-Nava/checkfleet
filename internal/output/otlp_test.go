package output

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOTLP(t *testing.T) {
	out, err := OTLP(fixtureResult())
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("invalid OTLP JSON: %v", err)
	}
	if _, ok := v["resourceMetrics"]; !ok {
		t.Error("missing resourceMetrics")
	}
	for _, want := range []string{
		"checkfleet.finding.status", "checkfleet.findings.total", "checkfleet.worst.status",
		"bad.example:443", // a target attribute
		`"asInt": "2"`,    // BAD severity as an int-string
		"service.name",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("OTLP output missing %q", want)
		}
	}
}
