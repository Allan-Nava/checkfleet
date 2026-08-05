package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/history"
)

// seedHistory writes a history next to a config path and returns that path.
// n runs, the last `down` of them with "web" BAD.
func seedHistory(t *testing.T, n, down int) string {
	t.Helper()
	cfg := filepath.Join(t.TempDir(), "checkfleet.yml")
	if err := os.WriteFile(cfg, []byte("checks: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := history.Open(historyPath(cfg))
	base := time.Now().Add(-time.Duration(n) * time.Hour)
	for i := 0; i < n; i++ {
		status := "OK"
		if i >= n-down {
			status = "BAD"
		}
		v := float64(10 + i)
		rec := history.Record{
			Unix: base.Add(time.Duration(i) * time.Hour).Unix(),
			Entries: []history.Entry{
				{Check: "http", Target: "web", Status: status},
				{Check: "redis", Target: "api", Status: "OK", Value: &v, Unit: "MB"},
			},
		}
		if err := store.Append(rec); err != nil {
			t.Fatal(err)
		}
	}
	return cfg
}

func TestInsightReturnsTheDefaultAnalyses(t *testing.T) {
	cfg := seedHistory(t, 20, 3)
	rep, err := (&App{}).Insight(InsightRequest{ConfigPath: cfg})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Runs != 20 {
		t.Errorf("runs = %d, want 20", rep.Runs)
	}
	if rep.Score == nil || rep.Digest == nil {
		t.Errorf("the zero request should ask for the zero-config analyses: %+v", rep)
	}
	if len(rep.Forecasts) != 0 || len(rep.Budgets) != 0 {
		t.Error("forecast and budget need values the operator did not supply")
	}
}

func TestInsightHonoursAnExplicitSelection(t *testing.T) {
	cfg := seedHistory(t, 20, 3)
	rep, err := (&App{}).Insight(InsightRequest{ConfigPath: cfg, Score: true})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Score == nil {
		t.Fatal("score was requested")
	}
	if rep.Digest != nil || len(rep.Recovery) != 0 {
		t.Errorf("an explicit selection must not also run the defaults: %+v", rep)
	}
}

func TestInsightAddsForecastAndBudgetOnlyWhenGiven(t *testing.T) {
	cfg := seedHistory(t, 20, 2)
	rep, err := (&App{}).Insight(InsightRequest{ConfigPath: cfg, Threshold: 100, SLO: 0.9})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Forecasts) == 0 {
		t.Error("a threshold should produce a forecast")
	}
	if len(rep.Budgets) == 0 {
		t.Error("an SLO should produce a budget")
	}
	// Out of range is ignored rather than dividing by zero.
	bad, err := (&App{}).Insight(InsightRequest{ConfigPath: cfg, SLO: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(bad.Budgets) != 0 {
		t.Error("SLO 1 must be ignored, not computed")
	}
}

// TestInsightUsesTheLiveRunForSingleRunAnalyses: clusters and the score should
// describe what is on screen, not the last record written to disk.
func TestInsightUsesTheLiveRunForSingleRunAnalyses(t *testing.T) {
	cfg := seedHistory(t, 20, 3)
	app := &App{}
	app.last = engine.Result{Findings: []engine.Finding{
		{Check: "http", Target: "web", Status: engine.OK},
		{Check: "redis", Target: "api", Status: engine.OK},
		{Check: "certs", Target: "c", Status: engine.OK},
	}}
	rep, err := app.Insight(InsightRequest{ConfigPath: cfg, Score: true})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Score.Findings != 3 {
		t.Errorf("score saw %d findings, want the 3 from the live run", rep.Score.Findings)
	}
}

// TestInsightWithNoHistoryIsNotAnError — a fresh config has nothing to analyse,
// and the UI should render "not enough history yet", not an error dialog.
func TestInsightWithNoHistoryIsNotAnError(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "checkfleet.yml")
	if err := os.WriteFile(cfg, []byte("checks: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := (&App{}).Insight(InsightRequest{ConfigPath: cfg})
	if err != nil {
		t.Fatalf("an absent history must not be an error: %v", err)
	}
	if !rep.Empty() || rep.Runs != 0 {
		t.Errorf("want an empty report, got %+v", rep)
	}
}

func TestInsightWithNoConfigPathIsInert(t *testing.T) {
	rep, err := (&App{}).Insight(InsightRequest{})
	if err != nil || !rep.Empty() {
		t.Errorf("no config path should yield an empty report, got %+v / %v", rep, err)
	}
}

func TestInsightWindowDefaultsAndCaps(t *testing.T) {
	cfg := seedHistory(t, 30, 0)
	rep, err := (&App{}).Insight(InsightRequest{ConfigPath: cfg, Window: 10, Score: true})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Runs != 10 {
		t.Errorf("runs = %d, want the requested window of 10", rep.Runs)
	}
	def, err := (&App{}).Insight(InsightRequest{ConfigPath: cfg, Score: true})
	if err != nil {
		t.Fatal(err)
	}
	if def.Runs != 30 {
		t.Errorf("runs = %d, want all 30 under the default window", def.Runs)
	}
}
