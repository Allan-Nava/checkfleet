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
// The same analyses ride along with `check --history` (CF-173); this command
// exists for the ones that need a value only the operator knows (--threshold,
// --slo) and for interrogating a history without running anything.
//
// Exit-code semantics: not a check, so 0 on success, non-zero only on a
// systemic failure (unreadable history, bad flag).
func runInsight(args []string) error {
	fs := flag.NewFlagSet("insight", flag.ContinueOnError)
	histPath := fs.String("history", "", "JSONL history file written by `check --history` (required)")
	window := fs.Int("window", 60, "how many recent runs to analyse")
	digest := fs.Bool("digest", false, "narrative summary of what changed across the window")
	score := fs.Bool("score", false, "single 0-100 health index for the fleet, with a per-module breakdown")
	clusters := fs.Bool("clusters", false, "group the newest run's problems by the dimension they share")
	anomaly := fs.Bool("anomaly", false, "flag metrics deviating from their own recent baseline")
	recovery := fs.Bool("recovery", false, "MTTR per target and how long the current outage has lasted")
	forecast := fs.Bool("forecast", false, "project when each metric crosses --threshold")
	threshold := fs.Float64("threshold", 0, "forecast: value to project a crossing of (required for --forecast)")
	minR2 := fs.Float64("min-r2", 0.7, "forecast: suppress projections whose fit is weaker than this")
	zScore := fs.Float64("z", 3, "anomaly: deviations from the baseline before a metric is flagged")
	slo := fs.Float64("slo", 0, "error budget: target availability, e.g. 0.999 (enables the budget report)")
	fastWindow := fs.Float64("fast-window", 0.1, "error budget: share of the history treated as the recent burn window")
	flapChanges := fs.Int("flap-changes", 3, "status changes in the window before a target counts as unstable")
	output := fs.String("output", "text", "text|json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *histPath == "" {
		return fmt.Errorf("--history is required: insight reads the file `check --history` writes")
	}
	if !*forecast && !*anomaly && !*recovery && !*score && !*digest && !*clusters && *slo == 0 {
		return fmt.Errorf("nothing to do: pass --digest, --score, --clusters, --anomaly, --recovery, --forecast or --slo")
	}
	if *forecast && *threshold == 0 {
		return fmt.Errorf("--forecast needs --threshold: there is no crossing without a value to cross")
	}
	if *slo != 0 && (*slo <= 0 || *slo >= 1) {
		return fmt.Errorf("--slo must be between 0 and 1 exclusive, e.g. 0.999 for three nines")
	}

	records, err := history.Open(*histPath).Recent(*window)
	if err != nil {
		return fmt.Errorf("read history: %w", err)
	}
	if len(records) == 0 {
		return fmt.Errorf("no history in %s — run `check --history %s` first", *histPath, *histPath)
	}

	opts := insight.Options{
		Now: time.Now(), Digest: *digest, Score: *score, Clusters: *clusters,
		Anomaly: *anomaly, Recovery: *recovery,
		MinR2: *minR2, Z: *zScore, FastWindow: *fastWindow, FlapChanges: *flapChanges,
	}
	if *forecast {
		opts.Threshold = *threshold
	}
	opts.Objective = *slo

	rep := insight.Analyse(records, nil, opts)
	if *output == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rep)
	}
	fmt.Print(insight.Text(rep, insight.TextOptions{
		Threshold: *threshold, Objective: *slo, Z: *zScore,
	}))
	return nil
}
