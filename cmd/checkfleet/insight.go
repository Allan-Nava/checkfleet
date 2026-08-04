package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/history"
	"github.com/Allan-Nava/checkfleet/internal/insight"
)

// runInsight reads the recorded history and reports what it implies (M30).
// It runs no checks and touches no infrastructure: everything here is derived
// from the JSONL that `check --history` already writes.
//
// Exit-code semantics: not a check, so 0 on success, non-zero only on a
// systemic failure (unreadable history, bad flag).
func runInsight(args []string) error {
	fs := flag.NewFlagSet("insight", flag.ContinueOnError)
	histPath := fs.String("history", "", "JSONL history file written by `check --history` (required)")
	window := fs.Int("window", 60, "how many recent runs to analyse")
	threshold := fs.Float64("threshold", 0, "forecast: value to project a crossing of (required for --forecast)")
	forecast := fs.Bool("forecast", false, "project when each metric crosses --threshold")
	anomaly := fs.Bool("anomaly", false, "flag metrics deviating from their own recent baseline")
	zScore := fs.Float64("z", 3, "anomaly: deviations from the baseline before a metric is flagged")
	recovery := fs.Bool("recovery", false, "MTTR per target and how long the current outage has lasted")
	slo := fs.Float64("slo", 0, "error budget: target availability, e.g. 0.999 (enables the budget report)")
	fastFraction := fs.Float64("fast-window", 0.1, "error budget: share of the history treated as the recent burn window")
	minR2 := fs.Float64("min-r2", 0.7, "forecast: suppress projections whose fit is weaker than this")
	output := fs.String("output", "text", "text|json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *histPath == "" {
		return fmt.Errorf("--history is required: insight reads the file `check --history` writes")
	}
	if !*forecast && !*anomaly && !*recovery && *slo == 0 {
		return fmt.Errorf("nothing to do: pass --forecast, --anomaly, --recovery or --slo")
	}
	if *slo != 0 && (*slo <= 0 || *slo >= 1) {
		return fmt.Errorf("--slo must be between 0 and 1 exclusive, e.g. 0.999 for three nines")
	}
	if *forecast && *threshold == 0 {
		return fmt.Errorf("--forecast needs --threshold: there is no crossing without a value to cross")
	}

	records, err := history.Open(*histPath).Recent(*window)
	if err != nil {
		return fmt.Errorf("read history: %w", err)
	}
	if len(records) == 0 {
		return fmt.Errorf("no history in %s — run `check --history %s` first", *histPath, *histPath)
	}

	doc := map[string]any{"runs": len(records)}
	var fRows []forecastRow
	var aRows []anomalyRow
	if *forecast {
		fRows = forecastRows(records, *threshold, *minR2, time.Now())
		doc["forecasts"] = fRows
	}
	if *anomaly {
		aRows = anomalyRows(records, *zScore)
		doc["anomalies"] = aRows
	}
	var rRows []recoveryRow
	if *recovery {
		rRows = recoveryRows(records, time.Now())
		doc["recovery"] = rRows
	}
	var bRows []budgetRow
	if *slo != 0 {
		bRows = budgetRows(records, *slo, *fastFraction, time.Now())
		doc["budgets"] = bRows
	}
	if *output == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(doc)
	}
	if *forecast {
		printForecasts(fRows, *threshold, len(records))
	}
	if *anomaly {
		if *forecast {
			fmt.Println()
		}
		printAnomalies(aRows, *zScore, len(records))
	}
	if *recovery {
		if *forecast || *anomaly {
			fmt.Println()
		}
		printRecovery(rRows, len(records))
	}
	if *slo != 0 {
		if *forecast || *anomaly || *recovery {
			fmt.Println()
		}
		printBudgets(bRows, *slo, len(records))
	}
	return nil
}

// recoveryRow is one target's outage history and current state.
type recoveryRow struct {
	Check      string `json:"check"`
	Target     string `json:"target"`
	Outages    int    `json:"outages"`
	MeanSec    int64  `json:"mttr_seconds,omitempty"`
	P50Sec     int64  `json:"p50_seconds,omitempty"`
	P90Sec     int64  `json:"p90_seconds,omitempty"`
	Down       bool   `json:"down"`
	OngoingSec int64  `json:"ongoing_seconds,omitempty"`
	Unresolved bool   `json:"started_before_window,omitempty"`
}

func recoveryRows(records []history.Record, now time.Time) []recoveryRow {
	var out []recoveryRow
	for _, s := range insight.StatusSeriesFrom(records) {
		r := insight.Recoveries(s, now)
		if len(r.Outages) == 0 && !r.Down {
			continue // a target that has never been down has nothing to say here
		}
		out = append(out, recoveryRow{
			Check: s.Check, Target: s.Target,
			Outages: len(r.Outages),
			MeanSec: int64(r.Mean.Seconds()), P50Sec: int64(r.P50.Seconds()), P90Sec: int64(r.P90.Seconds()),
			Down: r.Down, OngoingSec: int64(r.Ongoing.Seconds()), Unresolved: r.Unresolved,
		})
	}
	return out
}

func printRecovery(rows []recoveryRow, runs int) {
	if len(rows) == 0 {
		fmt.Printf("No target went down in the last %d run(s).\n", runs)
		return
	}
	fmt.Printf("Recovery, over %d run(s):\n\n", runs)
	for _, r := range rows {
		fmt.Printf("  %-10s %-38s", r.Check, r.Target)
		if r.Down {
			since := time.Duration(r.OngoingSec) * time.Second
			if r.Unresolved {
				fmt.Printf("  down for at least %s (started before the window)", since.Round(time.Minute))
			} else {
				fmt.Printf("  down for %s", since.Round(time.Minute))
			}
			if r.Outages > 0 {
				fmt.Printf(", usually back in ~%s", (time.Duration(r.MeanSec) * time.Second).Round(time.Minute))
			}
			fmt.Println()
			continue
		}
		fmt.Printf("  up · %d outage(s), MTTR ~%s (p50 %s, p90 %s)\n", r.Outages,
			(time.Duration(r.MeanSec) * time.Second).Round(time.Minute),
			(time.Duration(r.P50Sec) * time.Second).Round(time.Minute),
			(time.Duration(r.P90Sec) * time.Second).Round(time.Minute))
	}
}

// budgetRow is one target's error budget and burn rate.
type budgetRow struct {
	Check        string  `json:"check"`
	Target       string  `json:"target"`
	Availability float64 `json:"availability"`
	Consumed     float64 `json:"budget_consumed"`
	Remaining    float64 `json:"budget_remaining"`
	FastBurn     float64 `json:"fast_burn"`
	SlowBurn     float64 `json:"slow_burn"`
	Exhausted    string  `json:"exhausted,omitempty"`
	Samples      int     `json:"samples"`
	Note         string  `json:"note,omitempty"`
}

func budgetRows(records []history.Record, objective, fastFraction float64, now time.Time) []budgetRow {
	var out []budgetRow
	for _, s := range insight.StatusSeriesFrom(records) {
		b, ok := insight.ErrorBudget(s, objective, fastFraction, now)
		row := budgetRow{
			Check: s.Check, Target: s.Target,
			Availability: b.Availability, Consumed: b.Consumed, Remaining: b.Remaining,
			FastBurn: b.FastBurn, SlowBurn: b.SlowBurn, Samples: b.Samples,
		}
		switch {
		case !ok:
			row.Note = fmt.Sprintf("needs at least %d runs", insight.MinBudgetSamples)
		case b.Remaining == 0:
			row.Note = "budget exhausted — the objective is missed for this window"
		case !b.Exhausted.IsZero():
			row.Exhausted = b.Exhausted.Format(time.RFC3339)
		}
		out = append(out, row)
	}
	return out
}

func printBudgets(rows []budgetRow, objective float64, runs int) {
	fmt.Printf("Error budget against %.4g%% availability, over %d run(s):\n\n", objective*100, runs)
	for _, r := range rows {
		fmt.Printf("  %-10s %-38s %6.2f%% up", r.Check, r.Target, r.Availability*100)
		switch {
		case r.Note != "":
			fmt.Printf("  %s\n", r.Note)
		case r.Exhausted != "":
			fmt.Printf("  %.0f%% of budget left, fast burn %.1fx → gone %s\n", r.Remaining*100, r.FastBurn, r.Exhausted)
		default:
			fmt.Printf("  %.0f%% of budget left (burn %.2fx)\n", r.Remaining*100, r.SlowBurn)
		}
	}
}

// anomalyRow is one metric measured against its own recent normal.
type anomalyRow struct {
	Check     string  `json:"check"`
	Target    string  `json:"target"`
	Unit      string  `json:"unit,omitempty"`
	Latest    float64 `json:"latest"`
	Baseline  float64 `json:"baseline"`
	Deviation float64 `json:"deviation"`
	Z         float64 `json:"z"`
	Ratio     float64 `json:"ratio,omitempty"`
	Deviating bool    `json:"deviating"`
	Samples   int     `json:"samples"`
	Note      string  `json:"note,omitempty"`
}

func anomalyRows(records []history.Record, z float64) []anomalyRow {
	var out []anomalyRow
	for _, s := range insight.SeriesFrom(records) {
		a, dev := insight.Deviating(s, z)
		row := anomalyRow{
			Check: s.Check, Target: s.Target, Unit: s.Unit,
			Latest: a.Latest, Baseline: a.Baseline, Deviation: a.Deviation,
			Z: a.Z, Ratio: a.Ratio, Deviating: dev, Samples: a.Samples,
		}
		if a.Samples < insight.MinAnomalySamples {
			row.Latest = s.Points[len(s.Points)-1].Value
			row.Note = "not enough history for a baseline"
		}
		out = append(out, row)
	}
	return out
}

func printAnomalies(rows []anomalyRow, z float64, runs int) {
	if len(rows) == 0 {
		fmt.Printf("No metric series in the last %d run(s).\n", runs)
		return
	}
	fmt.Printf("Deviation from each metric's own baseline (z >= %g), over %d run(s):\n\n", z, runs)
	for _, r := range rows {
		fmt.Printf("  %-10s %-38s %8.2f%s", r.Check, r.Target, r.Latest, r.Unit)
		switch {
		case r.Note != "":
			fmt.Printf("  %s\n", r.Note)
		case r.Deviating && r.Ratio > 0:
			fmt.Printf("  %.1fx its norm of %.2f%s (z=%+.1f)\n", r.Ratio, r.Baseline, r.Unit, r.Z)
		case r.Deviating:
			fmt.Printf("  off its norm of %.2f%s (z=%+.1f)\n", r.Baseline, r.Unit, r.Z)
		default:
			fmt.Printf("  normal (baseline %.2f%s)\n", r.Baseline, r.Unit)
		}
	}
}

// forecastRow is one metric's projection, shaped for both renderers.
type forecastRow struct {
	Check   string  `json:"check"`
	Target  string  `json:"target"`
	Unit    string  `json:"unit,omitempty"`
	Latest  float64 `json:"latest"`
	Slope   float64 `json:"slope_per_day"`
	R2      float64 `json:"r2"`
	ETA     string  `json:"eta,omitempty"`
	InDays  float64 `json:"in_days,omitempty"`
	Samples int     `json:"samples"`
	// Why the row carries no ETA, when it does not: "flat", "receding",
	// "too few samples", "weak fit". Saying nothing would read as "no risk".
	Note string `json:"note,omitempty"`
}

func forecastRows(records []history.Record, threshold, minR2 float64, now time.Time) []forecastRow {
	var out []forecastRow
	for _, s := range insight.SeriesFrom(records) {
		latest := s.Points[len(s.Points)-1].Value
		row := forecastRow{
			Check: s.Check, Target: s.Target, Unit: s.Unit,
			Latest: latest, Samples: len(s.Points),
		}
		f, ok := insight.ETAToThreshold(s, threshold, now)
		row.Slope = f.Slope * 86400 // per day reads better than per second
		row.R2 = f.R2
		switch {
		case !ok:
			row.Note = "too few samples"
		case f.Due:
			// Trending the right way, and the fit puts the crossing before now —
			// usually a stale history. Saying "not trending" here would report no
			// risk on the target closest to the line.
			row.Note = "trend says it should already be over the threshold (history may be stale)"
		case !f.Crosses:
			row.Note = "not trending toward the threshold"
		case f.R2 < minR2:
			row.Note = fmt.Sprintf("weak fit (R²=%.2f) — projection suppressed", f.R2)
		default:
			row.ETA = f.ETA.Format(time.RFC3339)
			row.InDays = f.ETA.Sub(now).Hours() / 24
		}
		out = append(out, row)
	}
	return out
}

func printForecasts(rows []forecastRow, threshold float64, runs int) {
	if len(rows) == 0 {
		fmt.Printf("No metric series in the last %d run(s): no module recorded a numeric value.\n", runs)
		return
	}
	fmt.Printf("Forecast to %g, over %d run(s):\n\n", threshold, runs)
	for _, r := range rows {
		fmt.Printf("  %-10s %-38s %8.2f%s", r.Check, r.Target, r.Latest, r.Unit)
		if r.ETA != "" {
			fmt.Printf("  crosses in ~%.1f days (%+.2f%s/day, R²=%.2f)\n", r.InDays, r.Slope, r.Unit, r.R2)
			continue
		}
		fmt.Printf("  %s\n", r.Note)
	}
}
