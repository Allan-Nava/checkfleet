package output

import (
	"strings"
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

func TestSelfMetrics(t *testing.T) {
	res := engine.Result{
		Started:  time.Unix(1700000000, 0),
		Duration: 1500 * time.Millisecond,
		Findings: []engine.Finding{
			{Check: "http", Target: "a", Status: engine.OK},
			{Check: "http", Target: "b", Status: engine.ERROR},
			{Check: "certs", Target: "c", Status: engine.WARN},
		},
	}
	out := SelfMetrics(res)
	for _, want := range []string{
		`checkfleet_module_findings{module="http"} 2`,
		`checkfleet_module_errors{module="http"} 1`,
		`checkfleet_module_errors{module="certs"} 0`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	// run_duration / last_run_timestamp are emitted by Prometheus(), not here —
	// avoid duplicate metric families on the scrape.
	if strings.Contains(out, "checkfleet_run_duration_seconds") ||
		strings.Contains(out, "checkfleet_last_run_timestamp_seconds") {
		t.Errorf("SelfMetrics must not duplicate Prometheus() metrics:\n%s", out)
	}
}
