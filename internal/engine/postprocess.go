package engine

import "time"

// PostProcess applies everything that happens to a run's findings *after* the
// modules have produced them and *before* anything renders or gates on them
// (CF-163): maintenance windows first, then operator hints.
//
// The order is not incidental. Maintenance can drop a finding entirely, so
// annotating first would spend work on rows nobody will see — and, worse, would
// let a muted finding carry a runbook into a sink that only looks at hints.
//
// This function exists because the steps were previously open-coded at four
// call sites, and the desktop had drifted: it applied the hints but not the
// windows, so the same config produced a different verdict depending on which
// interface you opened it in. The point is less the deduplication than the
// parity test that guards it (TestPostProcessIsTheOnlyPipeline): a step added
// here reaches every interface at once, and a step added anywhere else fails.
//
// It does not mutate res.Findings' backing array; callers get a new slice.
func PostProcess(res Result, cfg *Config, now time.Time) Result {
	if cfg == nil {
		return res
	}
	res.Findings = ApplyMaintenance(res.Findings, cfg.Maintenance, now)
	res.Findings = ApplyRunbooks(res.Findings, cfg.Runbooks)
	return res
}
