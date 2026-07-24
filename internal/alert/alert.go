// Package alert turns findings into trigger/resolve events for on-call tools
// (PagerDuty, Opsgenie). The planning and payload building are pure and tested;
// the HTTP posting lives in the CLI. Dedup key is check/target, so repeated
// runs update the same alert and recoveries resolve it.
package alert

import (
	"encoding/json"
	"sort"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// Key is the dedup key for a finding: check/target.
func Key(f engine.Finding) string { return f.Check + "/" + f.Target }

// Event is a provider-agnostic alert intent.
type Event struct {
	Action   string        // "trigger" or "resolve"
	DedupKey string        // check/target
	Summary  string        // human-readable summary (trigger only)
	Severity engine.Status // BAD or ERROR (trigger only)
}

// Plan builds the events for a run: trigger for each current BAD/ERROR finding,
// resolve for every previously-open key that is no longer a problem.
func Plan(curr []engine.Finding, prevProblemKeys []string) []Event {
	problems := map[string]engine.Finding{}
	for _, f := range curr {
		if f.Status == engine.BAD || f.Status == engine.ERROR {
			problems[Key(f)] = f
		}
	}
	var events []Event
	keys := make([]string, 0, len(problems))
	for k := range problems {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		f := problems[k]
		events = append(events, Event{Action: "trigger", DedupKey: k, Summary: k + ": " + f.Message, Severity: f.Status})
	}
	for _, k := range prevProblemKeys {
		if _, still := problems[k]; !still {
			events = append(events, Event{Action: "resolve", DedupKey: k})
		}
	}
	return events
}

// pdSeverity maps a finding status to a PagerDuty severity.
func pdSeverity(s engine.Status) string {
	if s == engine.ERROR {
		return "critical"
	}
	return "error"
}

// PagerDutyPayload builds an Events API v2 payload for an event.
func PagerDutyPayload(routingKey, source string, e Event) (string, error) {
	m := map[string]any{
		"routing_key":  routingKey,
		"event_action": e.Action,
		"dedup_key":    e.DedupKey,
	}
	if e.Action == "trigger" {
		m["payload"] = map[string]any{
			"summary":  e.Summary,
			"source":   source,
			"severity": pdSeverity(e.Severity),
		}
	}
	b, err := json.Marshal(m)
	return string(b), err
}

// OpsgenieCreatePayload builds the body to create an Opsgenie alert (trigger).
// Resolve is done by the CLI via the close endpoint (alias = dedup key).
func OpsgenieCreatePayload(e Event) (string, error) {
	priority := "P2"
	if e.Severity == engine.ERROR {
		priority = "P1"
	}
	b, err := json.Marshal(map[string]any{
		"message":  e.Summary,
		"alias":    e.DedupKey,
		"priority": priority,
	})
	return string(b), err
}
