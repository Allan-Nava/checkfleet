package output

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

func sarifResultSet() engine.Result {
	return engine.Result{
		Findings: []engine.Finding{
			{Check: "http", Target: "https://example.com/", Status: engine.ERROR, Message: "dial tcp: timeout"},
			{Check: "certs", Target: "api.example.com:443", Status: engine.BAD, Message: "expires in 3 days",
				Value: engine.Num(3), Unit: "days"},
			{Check: "certs", Target: "www.example.com:443", Status: engine.WARN, Message: "expires in 20 days"},
			{Check: "http", Target: "https://example.com/ok", Status: engine.OK, Message: "HTTP 200"},
		},
		Labels: map[string]string{"env": "prod"},
	}
}

// parse the document so the tests assert on structure, not on formatting.
func parseSARIF(t *testing.T, s string) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(s), &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, s)
	}
	return doc
}

func TestSARIFEnvelope(t *testing.T) {
	out, err := SARIF(sarifResultSet(), SARIFOptions{Version: "1.2.3", ConfigPath: "checkfleet.yml"})
	if err != nil {
		t.Fatal(err)
	}
	doc := parseSARIF(t, out)

	if doc["version"] != "2.1.0" {
		t.Errorf("version = %v, want 2.1.0", doc["version"])
	}
	if doc["$schema"] != sarifSchema {
		t.Errorf("$schema = %v, want %s", doc["$schema"], sarifSchema)
	}
	runs, _ := doc["runs"].([]any)
	if len(runs) != 1 {
		t.Fatalf("got %d runs, want 1", len(runs))
	}
	run := runs[0].(map[string]any)
	driver := run["tool"].(map[string]any)["driver"].(map[string]any)
	if driver["name"] != "checkfleet" {
		t.Errorf("driver.name = %v", driver["name"])
	}
	if driver["version"] != "1.2.3" {
		t.Errorf("driver.version = %v, want the injected build version", driver["version"])
	}
}

// One rule per module that produced findings — not one per finding.
func TestSARIFRulesAreOnePerModule(t *testing.T) {
	out, err := SARIF(sarifResultSet(), SARIFOptions{})
	if err != nil {
		t.Fatal(err)
	}
	run := parseSARIF(t, out)["runs"].([]any)[0].(map[string]any)
	rules := run["tool"].(map[string]any)["driver"].(map[string]any)["rules"].([]any)

	if len(rules) != 2 {
		t.Fatalf("got %d rules, want 2 (certs, http) for 4 findings", len(rules))
	}
	// Sorted, so the document is reproducible for the same run.
	if id := rules[0].(map[string]any)["id"]; id != "checkfleet/certs" {
		t.Errorf("rules[0].id = %v, want checkfleet/certs (rules must be sorted)", id)
	}
	if desc := rules[0].(map[string]any)["shortDescription"].(map[string]any)["text"].(string); desc == "" {
		t.Error("rule has no shortDescription; it should come from moduledoc")
	}

	// ruleIndex must point at the matching rule, or consumers mislabel results.
	for _, r := range run["results"].([]any) {
		res := r.(map[string]any)
		idx := int(res["ruleIndex"].(float64))
		if idx < 0 || idx >= len(rules) {
			t.Fatalf("ruleIndex %d out of range", idx)
		}
		if got, want := rules[idx].(map[string]any)["id"], res["ruleId"]; got != want {
			t.Errorf("ruleIndex %d points at %v, but ruleId is %v", idx, got, want)
		}
	}
}

func TestSARIFLevelMapping(t *testing.T) {
	cases := map[engine.Status]string{
		engine.OK:    "none",
		engine.WARN:  "warning",
		engine.BAD:   "error",
		engine.ERROR: "error",
	}
	for status, want := range cases {
		if got := sarifLevel(status); got != want {
			t.Errorf("sarifLevel(%s) = %q, want %q", status, got, want)
		}
	}
}

// BAD and ERROR share the SARIF level "error", so the engine status must stay
// recoverable from properties — otherwise "unhealthy target" and "the check
// could not measure" become indistinguishable to a consumer.
func TestSARIFKeepsEngineStatusInProperties(t *testing.T) {
	out, err := SARIF(sarifResultSet(), SARIFOptions{})
	if err != nil {
		t.Fatal(err)
	}
	run := parseSARIF(t, out)["runs"].([]any)[0].(map[string]any)

	statuses := map[string]string{}
	for _, r := range run["results"].([]any) {
		res := r.(map[string]any)
		props := res["properties"].(map[string]any)
		statuses[props["target"].(string)] = props["status"].(string)
		if props["label.env"] != "prod" {
			t.Errorf("global labels missing from result properties: %v", props)
		}
	}
	if statuses["api.example.com:443"] != "BAD" {
		t.Errorf("BAD finding lost its status: %v", statuses)
	}
	if statuses["https://example.com/"] != "ERROR" {
		t.Errorf("ERROR finding lost its status: %v", statuses)
	}
}

func TestSARIFResultLocationsAnchorToConfig(t *testing.T) {
	out, err := SARIF(sarifResultSet(), SARIFOptions{ConfigPath: "deploy/checkfleet.yml"})
	if err != nil {
		t.Fatal(err)
	}
	run := parseSARIF(t, out)["runs"].([]any)[0].(map[string]any)
	for _, r := range run["results"].([]any) {
		res := r.(map[string]any)
		loc := res["locations"].([]any)[0].(map[string]any)["physicalLocation"].(map[string]any)
		if uri := loc["artifactLocation"].(map[string]any)["uri"]; uri != "deploy/checkfleet.yml" {
			t.Errorf("artifactLocation.uri = %v, want the config path", uri)
		}
		// GitHub drops results without a region.
		if line := loc["region"].(map[string]any)["startLine"].(float64); line < 1 {
			t.Errorf("startLine = %v, want >= 1", line)
		}
		// The target isn't in the location, so it has to be in the message.
		if msg := res["message"].(map[string]any)["text"].(string); !strings.Contains(msg, ":") {
			t.Errorf("message %q should carry the target", msg)
		}
	}
}

func TestSARIFDefaultsConfigPath(t *testing.T) {
	out, err := SARIF(sarifResultSet(), SARIFOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"uri": "checkfleet.yml"`) {
		t.Error("an empty ConfigPath should fall back to checkfleet.yml")
	}
}

// A fingerprint identifies the problem, not its current severity: a target
// going WARN → BAD must stay the same alert.
func TestSARIFFingerprintIsStableAcrossSeverity(t *testing.T) {
	warn := engine.Finding{Check: "certs", Target: "a:443", Status: engine.WARN, Message: "20 days"}
	bad := engine.Finding{Check: "certs", Target: "a:443", Status: engine.BAD, Message: "3 days"}
	other := engine.Finding{Check: "certs", Target: "b:443", Status: engine.WARN, Message: "20 days"}

	if fingerprint(warn) != fingerprint(bad) {
		t.Error("the same target changing severity produced a different fingerprint")
	}
	if fingerprint(warn) == fingerprint(other) {
		t.Error("different targets share a fingerprint")
	}
}

func TestSARIFEmptyRun(t *testing.T) {
	out, err := SARIF(engine.Result{}, SARIFOptions{})
	if err != nil {
		t.Fatal(err)
	}
	doc := parseSARIF(t, out)
	run := doc["runs"].([]any)[0].(map[string]any)
	// Must be [] and not null: a null results array is invalid SARIF.
	if results, ok := run["results"].([]any); !ok || len(results) != 0 {
		t.Errorf("empty run must serialise results as [], got %#v", run["results"])
	}
}
