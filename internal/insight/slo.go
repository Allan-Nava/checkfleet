package insight

import "time"

// Budget is an error budget and how fast it is being spent (CF-125). It answers
// the question a raw availability percentage does not: not "how healthy were we"
// but "at this rate, when do we run out".
//
// This is the *rate*, not the graph. Plotting uptime stays Grafana's job.
type Budget struct {
	// Objective is the target availability, 0..1 (0.999 for three nines).
	Objective float64
	// Availability observed over the whole window, 0..1.
	Availability float64
	// Budget is the share of allowed failure consumed so far, 0..1. Above 1 the
	// objective is already missed for this window.
	Consumed float64
	// Remaining is 1-Consumed, floored at zero.
	Remaining float64
	// FastBurn and SlowBurn are the burn rates over the short and long windows:
	// 1.0 means spending the budget exactly as fast as the window allows, 14.4
	// is the classic page-now threshold for a 1h window on a 30d objective.
	FastBurn float64
	SlowBurn float64
	// Exhausted is when the remaining budget runs out at the fast-burn rate.
	// Zero when nothing is burning, or when it is already gone.
	Exhausted time.Time
	// Samples is how many runs the window covered.
	Samples int
}

// MinBudgetSamples is the shortest history worth computing a budget over. A
// handful of runs makes every blip look like a 100% burn rate.
const MinBudgetSamples = 10

// ErrorBudget computes the budget for one target's status history against an
// objective (0..1). A run counts as failed when its status is BAD or ERROR:
// WARN is a warning, not downtime, and counting it as such would make every
// objective unmeetable.
//
// fastFraction is the share of the window treated as the "recent" burn window
// (0.1 = the last tenth of the runs). ok=false when the history is too short.
func ErrorBudget(s StatusSeries, objective, fastFraction float64, now time.Time) (Budget, bool) {
	n := len(s.Points)
	if n < MinBudgetSamples || objective <= 0 || objective >= 1 {
		return Budget{Samples: n}, false
	}
	b := Budget{Objective: objective, Samples: n}

	failed := 0
	for _, p := range s.Points {
		if isDown(p.Status) {
			failed++
		}
	}
	b.Availability = 1 - float64(failed)/float64(n)

	allowed := 1 - objective // the share of runs that may fail
	b.Consumed = (float64(failed) / float64(n)) / allowed
	b.Remaining = 1 - b.Consumed
	// Floor at zero with a tolerance: an exactly-spent budget comes out of the
	// division as ~1e-16 of headroom, and reporting "budget left" on a target
	// that has just used all of it is the wrong side to be wrong on.
	if b.Remaining < 1e-9 {
		b.Remaining = 0
	}
	b.SlowBurn = (float64(failed) / float64(n)) / allowed

	// Fast window: the most recent slice of the history.
	fastN := int(float64(n) * fastFraction)
	if fastN < 1 {
		fastN = 1
	}
	recent := s.Points[n-fastN:]
	recentFailed := 0
	for _, p := range recent {
		if isDown(p.Status) {
			recentFailed++
		}
	}
	b.FastBurn = (float64(recentFailed) / float64(fastN)) / allowed

	if b.FastBurn > 0 && b.Remaining > 0 {
		// At the fast rate, the remaining budget covers this fraction of the
		// window; project it forward over the window's real duration.
		span := time.Unix(s.Points[n-1].Unix, 0).Sub(time.Unix(s.Points[0].Unix, 0))
		if span > 0 {
			left := time.Duration(b.Remaining / b.FastBurn * float64(span))
			b.Exhausted = now.Add(left)
		}
	}
	return b, true
}

// isDown reports whether a recorded status counts against the error budget.
// WARN does not: it is a warning, and treating every warning as downtime makes
// any objective above two nines impossible to meet by construction.
func isDown(status string) bool { return status == "BAD" || status == "ERROR" }
