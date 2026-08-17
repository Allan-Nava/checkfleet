package alert

import (
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

func trigger(check, target string, s engine.Status) Event {
	return Event{Action: "trigger", DedupKey: check + "/" + target, Check: check, Target: target, Severity: s}
}

var routes = []engine.AlertRoute{
	{Check: "postgres", Provider: "pagerduty", KeyEnv: "PD_DBA"},
	{Check: "haproxy", Provider: "opsgenie", KeyEnv: "OG_NET"},
	{MinSeverity: "error", Provider: "pagerduty", KeyEnv: "PD_ONCALL"},
	{Provider: "sns", SNSTopicARN: "arn:aws:sns:eu-west-1:1:fleet"}, // catch-all
}

func TestFirstMatchingRouteWins(t *testing.T) {
	r, ok := Match(routes, trigger("postgres", "db-01:5432", engine.BAD), nil)
	if !ok || r.KeyEnv != "PD_DBA" {
		t.Errorf("postgres should reach the DBA route, got %+v (%v)", r, ok)
	}
	r, _ = Match(routes, trigger("haproxy", "lb-01:8404", engine.BAD), nil)
	if r.KeyEnv != "OG_NET" {
		t.Errorf("haproxy should reach the network route, got %+v", r)
	}
}

func TestSeverityRouteCatchesWhatTheSpecificOnesMiss(t *testing.T) {
	r, _ := Match(routes, trigger("redis", "cache-01", engine.ERROR), nil)
	if r.KeyEnv != "PD_ONCALL" {
		t.Errorf("an ERROR with no module rule should reach on-call, got %+v", r)
	}
	// A BAD on the same module falls through to the catch-all instead.
	r, _ = Match(routes, trigger("redis", "cache-01", engine.BAD), nil)
	if r.Provider != "sns" {
		t.Errorf("a BAD should fall through to the catch-all, got %+v", r)
	}
}

// TestAResolveIsNeverFilteredBySeverity — a resolve carries no severity because
// it is the *end* of a problem. Letting min_severity swallow it would route the
// trigger to a team and leave the alert open there forever.
func TestAResolveIsNeverFilteredBySeverity(t *testing.T) {
	only := []engine.AlertRoute{{MinSeverity: "error", Provider: "pagerduty", KeyEnv: "PD"}}
	r, ok := Match(only, Event{Action: "resolve", DedupKey: "redis/cache", Check: "redis", Target: "cache"}, nil)
	if !ok || r.KeyEnv != "PD" {
		t.Errorf("a resolve must still match the route its trigger did, got %+v (%v)", r, ok)
	}
}

func TestLabelsMustAllMatch(t *testing.T) {
	prodOnly := []engine.AlertRoute{
		{Labels: map[string]string{"env": "prod"}, Provider: "pagerduty", KeyEnv: "PD_PROD"},
	}
	if _, ok := Match(prodOnly, trigger("redis", "c", engine.BAD), map[string]string{"env": "staging"}); ok {
		t.Error("a staging run must not match a prod-only route")
	}
	if _, ok := Match(prodOnly, trigger("redis", "c", engine.BAD), map[string]string{"env": "prod"}); !ok {
		t.Error("a prod run should match")
	}
	// Every asked-for label has to be there, not just one of them.
	two := []engine.AlertRoute{{Labels: map[string]string{"env": "prod", "region": "eu"}, Provider: "sns"}}
	if _, ok := Match(two, trigger("redis", "c", engine.BAD), map[string]string{"env": "prod"}); ok {
		t.Error("a partial label match must not route")
	}
}

func TestTargetGlobRouting(t *testing.T) {
	byTarget := []engine.AlertRoute{
		{Target: "db-*", Provider: "pagerduty", KeyEnv: "PD_DBA"},
		{Provider: "sns"},
	}
	r, _ := Match(byTarget, trigger("tcp", "db-07:22", engine.BAD), nil)
	if r.KeyEnv != "PD_DBA" {
		t.Errorf("db-07 should match db-*, got %+v", r)
	}
	r, _ = Match(byTarget, trigger("tcp", "web-01:80", engine.BAD), nil)
	if r.Provider != "sns" {
		t.Errorf("web-01 should fall through, got %+v", r)
	}
}

// TestNoRoutesMeansNoMatch — the caller then keeps its flags, so a config
// without alert_routes behaves exactly as before.
func TestNoRoutesMeansNoMatch(t *testing.T) {
	if _, ok := Match(nil, trigger("redis", "c", engine.BAD), nil); ok {
		t.Error("an empty route list must not match anything")
	}
}

// TestAnEventMatchingNothingIsReportedNotGuessed: with rules present, silence
// would deliver a database alert to whoever happens to be first in the list.
func TestAnEventMatchingNothingIsReportedNotGuessed(t *testing.T) {
	strict := []engine.AlertRoute{{Check: "postgres", Provider: "pagerduty", KeyEnv: "PD"}}
	if _, ok := Match(strict, trigger("redis", "c", engine.BAD), nil); ok {
		t.Error("an unmatched event must report no route rather than borrow one")
	}
}

func TestSplitKeyHandlesTargetsContainingSlashes(t *testing.T) {
	check, target := splitKey("http/https://a.example/health")
	if check != "http" || target != "https://a.example/health" {
		t.Errorf("split = %q / %q", check, target)
	}
	if c, tg := splitKey("certs"); c != "certs" || tg != "" {
		t.Errorf("a key with no separator = %q / %q", c, tg)
	}
}

// TestPlanCarriesRoutingInputs — routing needs check and target on both actions,
// and a resolve has no finding left to read them from.
func TestPlanCarriesRoutingInputs(t *testing.T) {
	curr := []engine.Finding{{Check: "postgres", Target: "db-01:5432", Status: engine.BAD, Message: "down"}}
	events := Plan(curr, []string{"redis/cache-01"})
	for _, e := range events {
		if e.Check == "" {
			t.Errorf("%s event has no check: %+v", e.Action, e)
		}
		if e.Target == "" {
			t.Errorf("%s event has no target: %+v", e.Action, e)
		}
	}
}
