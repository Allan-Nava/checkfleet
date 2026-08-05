package main

import (
	"time"

	"github.com/Allan-Nava/checkfleet/internal/history"
	"github.com/Allan-Nava/checkfleet/internal/insight"
)

// InsightRequest is what the frontend asks for (CF-164). Every field is
// optional: the zero value asks for the analyses that need nothing from the
// operator, which is what the dashboard loads on open.
//
// Threshold and SLO are deliberately not defaulted. A forecast needs a value
// worth crossing and a budget needs a promise; inventing either would put a
// number on screen that nobody chose, which is worse than an empty panel.
type InsightRequest struct {
	ConfigPath string  `json:"configPath"`
	Window     int     `json:"window"`
	Digest     bool    `json:"digest"`
	Score      bool    `json:"score"`
	Clusters   bool    `json:"clusters"`
	Anomaly    bool    `json:"anomaly"`
	Recovery   bool    `json:"recovery"`
	Threshold  float64 `json:"threshold"`
	SLO        float64 `json:"slo"`
	Z          float64 `json:"z"`
	MinR2      float64 `json:"minR2"`
}

// Insight runs the M30 analyses over the config's persisted history and returns
// the same insight.Report the CLI serialises (CF-164).
//
// No statistics happen in JavaScript: the frontend receives numbers already
// computed by the shared package, so an insight means the same thing in the GUI
// and on the command line. That is the whole point of the binding — a second
// implementation in JS would drift within a release.
//
// An absent or empty history is not an error: it returns an empty report, which
// the UI renders as "not enough history yet".
func (a *App) Insight(req InsightRequest) (insight.Report, error) {
	p := historyPath(req.ConfigPath)
	if p == "" {
		return insight.Report{}, nil
	}
	window := req.Window
	if window <= 0 {
		window = 60
	}
	records, err := history.Open(p).Recent(window)
	if err != nil {
		return insight.Report{}, err
	}
	if len(records) == 0 {
		return insight.Report{}, nil
	}

	opts := insight.DefaultOptions(time.Now())
	// An explicit request overrides the defaults; a request with no analysis
	// selected keeps them, so the dashboard can call this with just a path.
	if req.Digest || req.Score || req.Clusters || req.Anomaly || req.Recovery {
		opts.Digest, opts.Score = req.Digest, req.Score
		opts.Clusters, opts.Anomaly, opts.Recovery = req.Clusters, req.Anomaly, req.Recovery
	}
	if req.Threshold != 0 {
		opts.Threshold = req.Threshold
	}
	if req.SLO > 0 && req.SLO < 1 {
		opts.Objective = req.SLO
	}
	if req.Z != 0 {
		opts.Z = req.Z
	}
	if req.MinR2 != 0 {
		opts.MinR2 = req.MinR2
	}

	// The live run's findings when we have one, so the single-run analyses
	// (clusters, score) describe what is on screen rather than the last
	// persisted record.
	a.mu.Lock()
	findings := a.last.Findings
	a.mu.Unlock()

	return insight.Analyse(records, findings, opts), nil
}
