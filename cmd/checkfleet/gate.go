package main

import (
	"fmt"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// defaultExitCode is what a tripped gate returns unless --exit-code says
// otherwise. It stays 2 for backward compatibility with --exit-on-bad.
const defaultExitCode = 2

// gate decides the process exit code from a run.
//
// The semantics this preserves: a check that ran is a *success*, so a run exits
// 0 no matter what it found unless the operator asked for a gate. Systemic
// failures (unreadable config, unknown module) are a different path entirely —
// they return an error from main and never reach here.
type gate struct {
	threshold engine.Status // "" = no gate
	code      int           // exit code when the gate trips
}

// parseGate builds a gate from the two flags. exitOnBad is the legacy boolean:
// it is exactly --exit-on bad, and the explicit --exit-on wins if both are set.
func parseGate(exitOn string, exitOnBad bool, code int) (gate, error) {
	if code < 1 || code > 125 {
		// 0 would make the gate a no-op, and 126+ collide with the shell's own
		// "command not executable" / "killed by signal" range.
		return gate{}, fmt.Errorf("--exit-code must be between 1 and 125, got %d", code)
	}
	threshold, ok := engine.ParseStatus(exitOn)
	if !ok {
		return gate{}, fmt.Errorf("unknown --exit-on %q: want warn, bad or error", exitOn)
	}
	if threshold == engine.OK {
		// Gating on OK would fail every run, including all-green ones. That is
		// never what someone means, so refuse it instead of silently obeying.
		return gate{}, fmt.Errorf("--exit-on ok would fail every run: want warn, bad or error")
	}
	if threshold == "" && exitOnBad {
		threshold = engine.BAD
	}
	return gate{threshold: threshold, code: code}, nil
}

// withImpliedThreshold gives --fail-on-new a gate when the caller never set
// one. "Fail on new findings" without a severity has only one sensible
// reading, and the alternative — silently never failing — would leave a flag
// that looks like it works.
func (g gate) withImpliedThreshold(failOnNew bool) gate {
	if failOnNew && g.threshold == "" {
		g.threshold = engine.BAD
	}
	return g
}

// exitCode returns the code to exit with for a run whose worst finding is
// worst: 0 when no gate is set or the run stayed below the threshold.
func (g gate) exitCode(worst engine.Status) int {
	if g.threshold == "" {
		return 0
	}
	if engine.AtLeast(worst, g.threshold) {
		return g.code
	}
	return 0
}
