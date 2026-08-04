package output

import (
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// docPath is the compatibility contract these formats are declared in (CF-153).
const docPath = "../../docs/compatibility.md"

// contractResult is a result exercising every field the formats can emit, so a
// new field cannot slip past the key assertions below.
func contractResult() engine.Result {
	v := 42.5
	return engine.Result{
		Findings: []engine.Finding{
			{Check: "http", Target: "https://a.example", Status: engine.OK, Message: "200 in 12ms", Value: &v, Unit: "ms"},
			{Check: "certs", Target: "b.example:443", Status: engine.BAD, Message: "expires in 2 days",
				Runbook: "https://wiki.example/tls", Remediation: "Renew and reload"},
		},
		Started:  time.Unix(1700000000, 0).UTC(),
		Duration: 1500 * time.Millisecond,
		Labels:   map[string]string{"env": "prod"},
	}
}

func objectKeys(t *testing.T, raw []byte) []string {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func TestJSONTopLevelKeys(t *testing.T) {
	out, err := JSON(contractResult())
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(objectKeys(t, []byte(out)), ",")
	// These are what pipelines parse. Renaming one is a breaking change and must
	// go through the deprecation policy in the contract doc.
	if want := "duration_ns,findings,labels,schema,started,summary,worst"; got != want {
		t.Errorf("JSON top-level keys = %q, want %q — see %s before changing the shape", got, want, docPath)
	}
}

func TestJSONFindingKeys(t *testing.T) {
	out, err := JSON(contractResult())
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Findings []json.RawMessage `json:"findings"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Findings) != 2 {
		t.Fatalf("want 2 findings, got %d", len(doc.Findings))
	}
	// Every optional field is omitempty and must stay that way (absent, not
	// null): the first finding carries the metric and no hints, the second the
	// operator hints and no metric.
	if got, want := strings.Join(objectKeys(t, doc.Findings[0]), ","), "check,message,status,target,unit,value"; got != want {
		t.Errorf("finding with metric = %q, want %q (runbook/remediation must be omitted) — see %s", got, want, docPath)
	}
	if got, want := strings.Join(objectKeys(t, doc.Findings[1]), ","), "check,message,remediation,runbook,status,target"; got != want {
		t.Errorf("finding with hints = %q, want %q (value/unit must be omitted, not null)", got, want)
	}
}

func TestJSONSchemaVersionEmitted(t *testing.T) {
	out, err := JSON(contractResult())
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Schema int `json:"schema"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Schema != JSONSchemaVersion {
		t.Errorf("schema = %d, want %d", doc.Schema, JSONSchemaVersion)
	}
}

var metricLine = regexp.MustCompile(`(?m)^# TYPE (checkfleet_[a-z_]+) `)

// metricNames returns the metric names declared by a Prometheus exposition.
func metricNames(text string) []string {
	var names []string
	for _, m := range metricLine.FindAllStringSubmatch(text, -1) {
		names = append(names, m[1])
	}
	sort.Strings(names)
	return names
}

func TestPrometheusMetricNames(t *testing.T) {
	got := strings.Join(metricNames(Prometheus(contractResult())), ",")
	// Metric names end up in users' dashboards and alert rules: renaming one
	// breaks them silently (the query just returns no data).
	want := "checkfleet_finding_status,checkfleet_findings_total,checkfleet_last_run_timestamp_seconds,checkfleet_run_duration_seconds,checkfleet_worst_status"
	if got != want {
		t.Errorf("metric names = %q, want %q — see %s before renaming", got, want, docPath)
	}
}

// TestFormatsAreDocumented is the anti-drift gate: every JSON key and metric
// name the renderers emit must appear in the compatibility contract, so a
// format change cannot land without the doc being updated in the same commit.
func TestFormatsAreDocumented(t *testing.T) {
	raw, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatal(err)
	}
	doc := string(raw)

	out, err := JSON(contractResult())
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range objectKeys(t, []byte(out)) {
		if !strings.Contains(doc, "`"+key+"`") {
			t.Errorf("JSON key %q is not documented in %s", key, docPath)
		}
	}
	// Finding keys too: they are nested, so iterating only the top level let a
	// new per-finding field land undocumented — which is exactly what this gate
	// exists to stop.
	var doc2 struct {
		Findings []json.RawMessage `json:"findings"`
	}
	if err := json.Unmarshal([]byte(out), &doc2); err != nil {
		t.Fatal(err)
	}
	for _, f := range doc2.Findings {
		for _, key := range objectKeys(t, f) {
			if !strings.Contains(doc, "`"+key+"`") {
				t.Errorf("finding key %q is not documented in %s", key, docPath)
			}
		}
	}
	for _, name := range append(metricNames(Prometheus(contractResult())), metricNames(SelfMetrics(contractResult()))...) {
		if !strings.Contains(doc, name) {
			t.Errorf("metric %q is not documented in %s", name, docPath)
		}
	}
}
