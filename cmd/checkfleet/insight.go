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
	minR2 := fs.Float64("min-r2", 0.7, "forecast: suppress projections whose fit is weaker than this")
	output := fs.String("output", "text", "text|json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *histPath == "" {
		return fmt.Errorf("--history is required: insight reads the file `check --history` writes")
	}
	if !*forecast && !*anomaly {
		return fmt.Errorf("nothing to do: pass --forecast or --anomaly")
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
	return nil
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
