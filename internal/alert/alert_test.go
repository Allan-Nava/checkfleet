package alert

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

func TestPlan(t *testing.T) {
	curr := []engine.Finding{
		{Check: "certs", Target: "a", Status: engine.BAD, Message: "expired"},
		{Check: "http", Target: "b", Status: engine.ERROR, Message: "timeout"},
		{Check: "dns", Target: "c", Status: engine.OK, Message: "ok"},
	}
	events := Plan(curr, []string{"certs/a", "http/x"})

	var triggers, resolves []string
	for _, e := range events {
		switch e.Action {
		case "trigger":
			triggers = append(triggers, e.DedupKey)
		case "resolve":
			resolves = append(resolves, e.DedupKey)
		}
	}
	if len(triggers) != 2 {
		t.Errorf("want 2 triggers, got %v", triggers)
	}
	// http/x was open before, not a problem now → resolve; certs/a still open → no resolve.
	if len(resolves) != 1 || resolves[0] != "http/x" {
		t.Errorf("want resolve [http/x], got %v", resolves)
	}
}

func TestPagerDutyPayload(t *testing.T) {
	e := Event{Action: "trigger", DedupKey: "certs/a", Summary: "certs/a: expired", Severity: engine.ERROR}
	out, err := PagerDutyPayload("RK", "checkfleet", e)
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if v["event_action"] != "trigger" || v["dedup_key"] != "certs/a" {
		t.Errorf("bad payload: %s", out)
	}
	if !strings.Contains(out, "critical") {
		t.Errorf("ERROR should map to critical severity: %s", out)
	}

	// Resolve payload omits the payload block.
	r, _ := PagerDutyPayload("RK", "checkfleet", Event{Action: "resolve", DedupKey: "certs/a"})
	if strings.Contains(r, "\"payload\"") {
		t.Errorf("resolve should not carry a payload block: %s", r)
	}
}

func TestOpsgenieCreatePayload(t *testing.T) {
	out, err := OpsgenieCreatePayload(Event{Action: "trigger", DedupKey: "http/b", Summary: "http/b: timeout", Severity: engine.BAD})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"alias":"http/b"`) || !strings.Contains(out, "P2") {
		t.Errorf("bad opsgenie payload: %s", out)
	}
}
