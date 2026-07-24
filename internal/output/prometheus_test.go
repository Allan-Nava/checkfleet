package output

import (
	"strings"
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

func TestPrometheusSeverityAndRollup(t *testing.T) {
	res := resultFrom([]engine.Finding{
		{Check: "http", Target: "https://x/health", Status: engine.BAD, Message: "500"},
		{Check: "certs", Target: "x:443", Status: engine.OK, Message: "ok"},
		{Check: "certs", Target: "y:443", Status: engine.WARN, Message: "vicino"},
	})
	out := Prometheus(res)

	for _, want := range []string{
		`checkfleet_finding_status{check="http",target="https://x/health"} 2`,
		`checkfleet_finding_status{check="certs",target="x:443"} 0`,
		`checkfleet_finding_status{check="certs",target="y:443"} 1`,
		`checkfleet_findings_total{status="OK"} 1`,
		`checkfleet_findings_total{status="BAD"} 1`,
		`checkfleet_worst_status 2`,
		"# TYPE checkfleet_finding_status gauge",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output Prometheus manca la riga:\n%s\n---\n%s", want, out)
		}
	}
}

func TestPrometheusDedupWorstWins(t *testing.T) {
	// Same (check,target) twice: the worst severity must be the one exposed,
	// and only once (no duplicate series).
	res := resultFrom([]engine.Finding{
		{Check: "nats", Target: "gw", Status: engine.OK, Message: "a"},
		{Check: "nats", Target: "gw", Status: engine.BAD, Message: "b"},
	})
	out := Prometheus(res)
	if c := strings.Count(out, `checkfleet_finding_status{check="nats",target="gw"}`); c != 1 {
		t.Fatalf("duplicate series: want 1, got %d\n%s", c, out)
	}
	if !strings.Contains(out, `checkfleet_finding_status{check="nats",target="gw"} 2`) {
		t.Errorf("dedup dovrebbe tenere il worst (BAD=2):\n%s", out)
	}
}

func TestPrometheusEscapesLabels(t *testing.T) {
	res := resultFrom([]engine.Finding{
		{Check: "http", Target: `weird"back\slash`, Status: engine.OK, Message: "x"},
	})
	out := Prometheus(res)
	if !strings.Contains(out, `target="weird\"back\\slash"`) {
		t.Errorf("label non escappata correttamente:\n%s", out)
	}
}
