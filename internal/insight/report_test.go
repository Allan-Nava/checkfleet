package insight

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/history"
)

// runs builds n hourly records where target "web" is BAD in the last `down`
// of them, and "api" carries a rising metric.
func runs(n, down int) []history.Record {
	base := t0
	out := make([]history.Record, 0, n)
	for i := 0; i < n; i++ {
		status := "OK"
		if i >= n-down {
			status = "BAD"
		}
		v := float64(10 + i)
		out = append(out, history.Record{
			Unix: base.Add(time.Duration(i) * time.Hour).Unix(),
			Entries: []history.Entry{
				{Check: "http", Target: "web", Status: status},
				{Check: "redis", Target: "api", Status: "OK", Value: &v, Unit: "MB"},
			},
		})
	}
	return out
}

func TestDefaultOptionsAskNothingOfTheOperator(t *testing.T) {
	o := DefaultOptions(t0)
	if o.Threshold != 0 {
		t.Error("a default forecast would need a threshold nobody supplied")
	}
	if o.Objective != 0 {
		t.Error("a default budget would need an SLO nobody promised")
	}
	if !o.Score || !o.Digest || !o.Clusters || !o.Recovery {
		t.Errorf("the zero-config analyses should all be on: %+v", o)
	}
}

func TestAnalyseRunsOnlyWhatWasAsked(t *testing.T) {
	r := Analyse(runs(20, 3), nil, Options{Now: t0, Score: true})
	if r.Score == nil {
		t.Fatal("score was requested")
	}
	if r.Digest != nil || len(r.Recovery) != 0 || len(r.Anomalies) != 0 || len(r.Budgets) != 0 {
		t.Errorf("unrequested analyses ran: %+v", r)
	}
}

func TestForecastNeedsAThresholdAndBudgetAnObjective(t *testing.T) {
	rec := runs(20, 0)
	if r := Analyse(rec, nil, Options{Now: t0}); len(r.Forecasts) != 0 || len(r.Budgets) != 0 {
		t.Error("no threshold and no objective must produce neither")
	}
	r := Analyse(rec, nil, Options{Now: t0, Threshold: 100, Objective: 0.99})
	if len(r.Forecasts) == 0 {
		t.Error("a threshold should produce a forecast")
	}
	if len(r.Budgets) == 0 {
		t.Error("an objective should produce a budget")
	}
	// An out-of-range objective is ignored rather than dividing by zero.
	if bad := Analyse(rec, nil, Options{Now: t0, Objective: 1}); len(bad.Budgets) != 0 {
		t.Error("objective 1 must be rejected, not computed")
	}
}

// TestAnalyseFallsBackToTheNewestRecord: `insight` has no run in hand, so the
// single-run analyses must read the history's last record instead of nothing.
func TestAnalyseFallsBackToTheNewestRecord(t *testing.T) {
	r := Analyse(runs(20, 3), nil, Options{Now: t0, Score: true})
	if r.Score.Findings != 2 {
		t.Errorf("score saw %d findings, want the 2 in the newest record", r.Score.Findings)
	}
}

func TestAnalysePrefersTheSuppliedFindings(t *testing.T) {
	live := []engine.Finding{
		{Check: "http", Target: "web", Status: engine.OK},
		{Check: "redis", Target: "api", Status: engine.OK},
		{Check: "certs", Target: "c", Status: engine.OK},
	}
	r := Analyse(runs(20, 3), live, Options{Now: t0, Score: true})
	if r.Score.Findings != 3 {
		t.Errorf("score saw %d findings, want the 3 from the live run", r.Score.Findings)
	}
	if r.Score.Value != 100 {
		t.Errorf("an all-green live run should score 100, got %v", r.Score.Value)
	}
}

func TestEmptyHistoryProducesAnEmptyReport(t *testing.T) {
	r := Analyse(nil, nil, DefaultOptions(t0))
	if !r.Empty() || r.Runs != 0 {
		t.Errorf("want an empty report, got %+v", r)
	}
}

// TestReportSectionsAreOmittedWhenAbsent — the JSON rides inside `check`'s
// output, so an unasked analysis must not appear as null or [].
func TestReportSectionsAreOmittedWhenAbsent(t *testing.T) {
	raw, err := json.Marshal(Analyse(runs(20, 0), nil, Options{Now: t0, Score: true}))
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, key := range []string{"digest", "clusters", "anomalies", "recovery", "forecasts", "budgets"} {
		if strings.Contains(s, `"`+key+`"`) {
			t.Errorf("unrequested section %q leaked into the JSON: %s", key, s)
		}
	}
	if !strings.Contains(s, `"score"`) {
		t.Error("the requested section is missing")
	}
}

func TestTextRendersEverySectionOnce(t *testing.T) {
	r := Analyse(runs(20, 3), nil, Options{
		Now: t0, Score: true, Digest: true, Recovery: true, Anomaly: true,
		Threshold: 100, Objective: 0.9,
	})
	out := Text(r, TextOptions{Threshold: 100, Objective: 0.9, Z: 3})
	for _, want := range []string{"Fleet health", "Recovery", "Deviation", "Forecast to", "Error budget"} {
		if n := strings.Count(out, want); n != 1 {
			t.Errorf("section %q appears %d times, want 1:\n%s", want, n, out)
		}
	}
	// Blocks are separated by exactly one blank line, never two.
	if strings.Contains(out, "\n\n\n") {
		t.Errorf("double blank line between sections:\n%q", out)
	}
}

func TestTextOfAnEmptyReportIsEmpty(t *testing.T) {
	if got := Text(Report{}, TextOptions{}); got != "" {
		t.Errorf("an empty report should render nothing, got %q", got)
	}
}

func TestAnalyseIsDeterministic(t *testing.T) {
	rec := runs(20, 3)
	o := Options{Now: t0, Score: true, Digest: true, Clusters: true, Recovery: true, Anomaly: true}
	first, err := json.Marshal(Analyse(rec, nil, o))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		next, err := json.Marshal(Analyse(rec, nil, o))
		if err != nil {
			t.Fatal(err)
		}
		if string(next) != string(first) {
			t.Fatal("Analyse is not deterministic — map iteration leaked out")
		}
	}
}

// TestScoreCarriesItsTrend — an index without direction says much less than one
// with it, which is the whole reason CF-165 shows a strip and not a number.
func TestScoreCarriesItsTrend(t *testing.T) {
	r := Analyse(runs(20, 5), nil, Options{Now: t0, Score: true})
	if len(r.Score.Trend) != 20 {
		t.Fatalf("trend has %d points, want one per run", len(r.Score.Trend))
	}
	// The last runs are BAD, so the index must end lower than it started.
	if !(r.Score.Trend[len(r.Score.Trend)-1] < r.Score.Trend[0]) {
		t.Errorf("trend should fall as the fleet degrades: %v → %v",
			r.Score.Trend[0], r.Score.Trend[len(r.Score.Trend)-1])
	}
}

func TestSingleRunHasNoTrend(t *testing.T) {
	r := Analyse(runs(1, 0), nil, Options{Now: t0, Score: true})
	if len(r.Score.Trend) != 0 {
		t.Errorf("one run is not a trend: %v", r.Score.Trend)
	}
}
