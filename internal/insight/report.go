package insight

import (
	"fmt"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/history"
)

// Report is every analysis M30 can produce, in one JSON-shaped value (CF-173).
//
// It lives here rather than in the CLI because three consumers need the same
// shape: `checkfleet insight`, the `insight` block of `check --output json`, and
// the desktop bindings. Defining it once is what keeps them from drifting into
// three slightly different vocabularies for the same numbers.
//
// Every section is omitempty: a report carries only what was asked for and what
// the history could actually support.
type Report struct {
	Runs      int           `json:"runs"`
	Score     *ScoreReport  `json:"score,omitempty"`
	Digest    *Digest       `json:"digest,omitempty"`
	Clusters  []ClusterRow  `json:"clusters,omitempty"`
	Anomalies []AnomalyRow  `json:"anomalies,omitempty"`
	Recovery  []RecoveryRow `json:"recovery,omitempty"`
	Flapping  []FlappingRow `json:"flapping,omitempty"`
	Forecasts []ForecastRow `json:"forecasts,omitempty"`
	Budgets   []BudgetRow   `json:"budgets,omitempty"`
	Notes     []string      `json:"notes,omitempty"`
}

// Empty reports whether the analysis found nothing worth showing.
func (r Report) Empty() bool {
	return r.Score == nil && r.Digest == nil && len(r.Clusters) == 0 && len(r.Anomalies) == 0 &&
		len(r.Recovery) == 0 && len(r.Forecasts) == 0 && len(r.Budgets) == 0 && len(r.Flapping) == 0
}

// Options selects which analyses run and tunes them. The zero value asks for
// the ones that need no operator input; Threshold and Objective are opt-in
// because only the operator knows what value is worth crossing or promising.
type Options struct {
	Now time.Time

	Score    bool
	Digest   bool
	Clusters bool
	Anomaly  bool
	Recovery bool
	Flapping bool

	// Forecast runs only when Threshold is non-zero.
	Threshold float64
	MinR2     float64
	// Budget runs only when Objective is in (0,1).
	Objective  float64
	FastWindow float64

	Z           float64
	FlapChanges int
}

// DefaultOptions are the analyses that need nothing from the operator: the ones
// `check --history` can attach without asking a question first.
func DefaultOptions(now time.Time) Options {
	return Options{
		Now: now, Score: true, Digest: true, Clusters: true, Recovery: true, Flapping: true,
		Z: 3, FlapChanges: 3, MinR2: 0.7, FastWindow: 0.1,
	}
}

// ScoreReport is the fleet index with the breakdown that gives it somewhere to
// point.
type ScoreReport struct {
	Value    float64            `json:"value"`
	Findings int                `json:"findings"`
	Unstable int                `json:"unstable_targets"`
	Modules  map[string]float64 `json:"modules,omitempty"`
	Worst    []string           `json:"worst_modules,omitempty"`
	// Trend is the index for each run in the window, oldest first. The index
	// exists to be watched over time — a single instantaneous value says much
	// less than its direction — so the series ships with it.
	Trend []float64 `json:"trend,omitempty"`
}

// ClusterRow is one correlated-failure group.
type ClusterRow struct {
	Dimension string   `json:"dimension"`
	Value     string   `json:"value"`
	Size      int      `json:"size"`
	Targets   []string `json:"targets"`
}

// ForecastRow is one metric's projection toward a threshold.
type ForecastRow struct {
	Check   string  `json:"check"`
	Target  string  `json:"target"`
	Unit    string  `json:"unit,omitempty"`
	Latest  float64 `json:"latest"`
	Slope   float64 `json:"slope_per_day"`
	R2      float64 `json:"r2"`
	ETA     string  `json:"eta,omitempty"`
	InDays  float64 `json:"in_days,omitempty"`
	Samples int     `json:"samples"`
	Note    string  `json:"note,omitempty"`
}

// AnomalyRow is one metric measured against its own recent normal.
type AnomalyRow struct {
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

// BudgetRow is one target's error budget and burn rate.
type BudgetRow struct {
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

// FlappingRow is one target's oscillation score (CF-171).
type FlappingRow struct {
	Check   string  `json:"check"`
	Target  string  `json:"target"`
	Score   float64 `json:"score"`
	Recent  float64 `json:"recent"`
	Changes int     `json:"changes"`
	Runs    int     `json:"runs"`
	Level   string  `json:"level"`
}

// RecoveryRow is one target's outage history and current state.
type RecoveryRow struct {
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

// Analyse runs the selected analyses over the history, plus the current run's
// findings where an analysis works on a single run (clusters and the score).
//
// findings may be nil: the history's newest record is used instead, which is
// what `checkfleet insight` does when there is no run in hand.
func Analyse(records []history.Record, findings []engine.Finding, o Options) Report {
	r := Report{Runs: len(records)}
	if len(records) == 0 {
		return r
	}
	if o.Now.IsZero() {
		o.Now = time.Now()
	}
	metricSeries := SeriesFrom(records)
	statusSeries := StatusSeriesFrom(records)
	unstable := UnstableKeys(statusSeries, orDefault(o.FlapChanges, 3))

	if findings == nil {
		findings = findingsOf(records[len(records)-1])
	}

	if o.Clusters {
		for _, c := range Correlate(findings) {
			row := ClusterRow{Dimension: c.Dimension, Value: c.Value, Size: c.Size()}
			for _, f := range c.Findings {
				row.Targets = append(row.Targets, f.Check+" "+f.Target)
			}
			r.Clusters = append(r.Clusters, row)
		}
	}
	if o.Score {
		fleet := FleetScore(findings, unstable)
		mods := ModuleScores(findings, unstable)
		sr := &ScoreReport{
			Value: fleet.Value, Findings: fleet.Findings, Unstable: fleet.Unstable,
			Modules: make(map[string]float64, len(mods)),
		}
		for name, m := range mods {
			sr.Modules[name] = m.Value
		}
		for _, name := range SortedModules(mods) {
			if mods[name].Value < 100 {
				sr.Worst = append(sr.Worst, name)
			}
		}
		// Per-run trend. Computed on the history's own records (not the live
		// findings) so every point is measured the same way.
		if len(records) > 1 {
			sr.Trend = make([]float64, 0, len(records))
			for _, rec := range records {
				sr.Trend = append(sr.Trend, FleetScore(findingsOf(rec), unstable).Value)
			}
		}
		r.Score = sr
	}
	if o.Digest {
		d := Compare(statusSeries, orDefault(o.FlapChanges, 3))
		r.Digest = &d
	}
	if o.Recovery {
		for _, s := range statusSeries {
			rec := Recoveries(s, o.Now)
			if len(rec.Outages) == 0 && !rec.Down {
				continue
			}
			r.Recovery = append(r.Recovery, RecoveryRow{
				Check: s.Check, Target: s.Target, Outages: len(rec.Outages),
				MeanSec: secs(rec.Mean), P50Sec: secs(rec.P50), P90Sec: secs(rec.P90),
				Down: rec.Down, OngoingSec: secs(rec.Ongoing), Unresolved: rec.Unresolved,
			})
		}
	}
	if o.Flapping {
		for _, s := range statusSeries {
			f, ok := Flapping(s)
			if !ok || f.Score == 0 {
				continue // nothing to badge
			}
			r.Flapping = append(r.Flapping, FlappingRow{
				Check: s.Check, Target: s.Target,
				Score: f.Score, Recent: f.Recent, Changes: f.Changes, Runs: f.Runs, Level: f.Level(),
			})
		}
	}
	if o.Anomaly {
		z := o.Z
		if z == 0 {
			z = 3
		}
		for _, s := range metricSeries {
			a, dev := Deviating(s, z)
			row := AnomalyRow{
				Check: s.Check, Target: s.Target, Unit: s.Unit,
				Latest: a.Latest, Baseline: a.Baseline, Deviation: a.Deviation,
				Z: a.Z, Ratio: a.Ratio, Deviating: dev, Samples: a.Samples,
			}
			if a.Samples < MinAnomalySamples {
				row.Latest = s.Points[len(s.Points)-1].Value
				row.Note = "not enough history for a baseline"
			}
			r.Anomalies = append(r.Anomalies, row)
		}
	}
	if o.Threshold != 0 {
		minR2 := o.MinR2
		if minR2 == 0 {
			minR2 = 0.7
		}
		for _, s := range metricSeries {
			r.Forecasts = append(r.Forecasts, forecastRowOf(s, o.Threshold, minR2, o.Now))
		}
	}
	if o.Objective > 0 && o.Objective < 1 {
		fast := o.FastWindow
		if fast == 0 {
			fast = 0.1
		}
		for _, s := range statusSeries {
			r.Budgets = append(r.Budgets, budgetRowOf(s, o.Objective, fast, o.Now))
		}
	}
	if len(metricSeries) == 0 && (o.Anomaly || o.Threshold != 0) {
		r.Notes = append(r.Notes, "no module recorded a numeric value, so there is no metric to analyse")
	}
	return r
}

func forecastRowOf(s Series, threshold, minR2 float64, now time.Time) ForecastRow {
	latest := s.Points[len(s.Points)-1].Value
	row := ForecastRow{Check: s.Check, Target: s.Target, Unit: s.Unit, Latest: latest, Samples: len(s.Points)}
	f, ok := ETAToThreshold(s, threshold, now)
	row.Slope = f.Slope * 86400 // per day reads better than per second
	row.R2 = f.R2
	switch {
	case !ok:
		row.Note = "too few samples"
	case f.Due:
		row.Note = "trend says it should already be over the threshold (history may be stale)"
	case !f.Crosses:
		row.Note = "not trending toward the threshold"
	case f.R2 < minR2:
		row.Note = fmt.Sprintf("weak fit (R²=%.2f) — projection suppressed", f.R2)
	default:
		row.ETA = f.ETA.Format(time.RFC3339)
		row.InDays = f.ETA.Sub(now).Hours() / 24
	}
	return row
}

func budgetRowOf(s StatusSeries, objective, fast float64, now time.Time) BudgetRow {
	b, ok := ErrorBudget(s, objective, fast, now)
	row := BudgetRow{
		Check: s.Check, Target: s.Target,
		Availability: b.Availability, Consumed: b.Consumed, Remaining: b.Remaining,
		FastBurn: b.FastBurn, SlowBurn: b.SlowBurn, Samples: b.Samples,
	}
	switch {
	case !ok:
		row.Note = fmt.Sprintf("needs at least %d runs", MinBudgetSamples)
	case b.Remaining == 0:
		row.Note = "budget exhausted — the objective is missed for this window"
	case !b.Exhausted.IsZero():
		row.Exhausted = b.Exhausted.Format(time.RFC3339)
	}
	return row
}

// findingsOf reconstructs a run's findings from a history record. The record
// keeps status but not the message, which none of the analyses read.
func findingsOf(r history.Record) []engine.Finding {
	out := make([]engine.Finding, 0, len(r.Entries))
	for _, e := range r.Entries {
		out = append(out, engine.Finding{
			Check: e.Check, Target: e.Target, Status: engine.Status(e.Status),
			Value: e.Value, Unit: e.Unit,
		})
	}
	return out
}

func secs(d time.Duration) int64 { return int64(d.Seconds()) }

func orDefault(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}
