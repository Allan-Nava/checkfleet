package output

import (
	"encoding/json"
	"sort"
	"strconv"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// OTLP renders a run as an OTLP/HTTP metrics request in JSON encoding — the
// same gauges as the Prometheus output, ready to POST to an OpenTelemetry
// collector's /v1/metrics with Content-Type: application/json. Zero deps: the
// payload is hand-built (no OTel SDK). int64 values are strings per OTLP/JSON.
func OTLP(res engine.Result) (string, error) {
	ts := strconv.FormatInt(res.Started.UnixNano(), 10)

	str := func(s string) *string { return &s }
	kv := func(k, v string) otlpKV { return otlpKV{Key: k, Value: otlpValue{StringValue: str(v)}} }
	dp := func(attrs []otlpKV, val int) otlpDataPoint {
		return otlpDataPoint{Attributes: attrs, TimeUnixNano: ts, AsInt: strconv.Itoa(val)}
	}

	// Worst severity per (check,target), stable order.
	type key struct{ check, target string }
	worst := map[key]int{}
	var order []key
	for _, f := range res.Findings {
		k := key{f.Check, f.Target}
		s := severity[f.Status]
		if cur, ok := worst[k]; ok {
			if s > cur {
				worst[k] = s
			}
			continue
		}
		worst[k] = s
		order = append(order, k)
	}
	sort.Slice(order, func(i, j int) bool {
		if order[i].check != order[j].check {
			return order[i].check < order[j].check
		}
		return order[i].target < order[j].target
	})

	var findingPoints []otlpDataPoint
	for _, k := range order {
		findingPoints = append(findingPoints, dp([]otlpKV{kv("check", k.check), kv("target", k.target)}, worst[k]))
	}

	sum := engine.Summarize(res.Findings)
	var totalPoints []otlpDataPoint
	for _, st := range []engine.Status{engine.OK, engine.WARN, engine.BAD, engine.ERROR} {
		totalPoints = append(totalPoints, dp([]otlpKV{kv("status", string(st))}, sum[st]))
	}

	req := otlpRequest{ResourceMetrics: []otlpResourceMetrics{{
		Resource: otlpResource{Attributes: []otlpKV{kv("service.name", "checkfleet")}},
		ScopeMetrics: []otlpScopeMetrics{{
			Scope: otlpScope{Name: "checkfleet"},
			Metrics: []otlpMetric{
				{Name: "checkfleet.finding.status", Gauge: otlpGauge{DataPoints: findingPoints}},
				{Name: "checkfleet.findings.total", Gauge: otlpGauge{DataPoints: totalPoints}},
				{Name: "checkfleet.worst.status", Gauge: otlpGauge{DataPoints: []otlpDataPoint{
					dp(nil, severity[engine.Worst(res.Findings)]),
				}}},
			},
		}},
	}}}

	b, err := json.MarshalIndent(req, "", "  ")
	return string(b), err
}

type otlpValue struct {
	StringValue *string `json:"stringValue,omitempty"`
}
type otlpKV struct {
	Key   string    `json:"key"`
	Value otlpValue `json:"value"`
}
type otlpDataPoint struct {
	Attributes   []otlpKV `json:"attributes,omitempty"`
	TimeUnixNano string   `json:"timeUnixNano"`
	AsInt        string   `json:"asInt"`
}
type otlpGauge struct {
	DataPoints []otlpDataPoint `json:"dataPoints"`
}
type otlpMetric struct {
	Name  string    `json:"name"`
	Gauge otlpGauge `json:"gauge"`
}
type otlpScope struct {
	Name string `json:"name"`
}
type otlpScopeMetrics struct {
	Scope   otlpScope    `json:"scope"`
	Metrics []otlpMetric `json:"metrics"`
}
type otlpResource struct {
	Attributes []otlpKV `json:"attributes"`
}
type otlpResourceMetrics struct {
	Resource     otlpResource       `json:"resource"`
	ScopeMetrics []otlpScopeMetrics `json:"scopeMetrics"`
}
type otlpRequest struct {
	ResourceMetrics []otlpResourceMetrics `json:"resourceMetrics"`
}
