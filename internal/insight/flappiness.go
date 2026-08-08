package insight

import "math"

// Flappiness measures how much a target oscillates, not just whether it does
// (CF-171). CF-32 shipped the detection — a count of status changes against a
// threshold — and that count answers the wrong question on its own: four
// changes in the last 10 runs and four spread over 60 are the same number and
// very different problems.
//
// The score is the transition *rate* over the window, normalised so that
// alternating on every run scores 100.
type Flappiness struct {
	// Score is 0–100. 0 = never changed, 100 = changed at every single run.
	Score float64
	// Changes is the raw count, kept because it is what an operator quotes.
	Changes int
	// Runs is how many samples the score was computed over.
	Runs int
	// Recent is the score over the most recent third of the window, so a target
	// that has settled down reads differently from one that just started.
	Recent float64
}

// Level buckets the score for a badge. Thresholds are deliberately coarse: a
// badge exists to be glanced at, and a precise number on a chip invites reading
// a difference between 41 and 44 that the data does not support.
func (f Flappiness) Level() string {
	switch {
	case f.Score >= 50:
		return "high"
	case f.Score >= 20:
		return "medium"
	case f.Score > 0:
		return "low"
	default:
		return ""
	}
}

// MinFlapSamples is the shortest history worth scoring. With three samples a
// single blip reads as 50% flappiness, which is a number, not a signal.
const MinFlapSamples = 6

// Flapping scores one target's oscillation. ok=false when the history is too
// short to say anything.
func Flapping(s StatusSeries) (Flappiness, bool) {
	n := len(s.Points)
	if n < MinFlapSamples {
		return Flappiness{Runs: n}, false
	}
	f := Flappiness{Runs: n, Changes: transitions(s.Points)}
	// n samples allow at most n-1 transitions.
	f.Score = round1(float64(f.Changes) / float64(n-1) * 100)

	// The most recent third, so "was flapping last week, steady since" is
	// visible instead of averaged away.
	tail := n / 3
	if tail >= 2 {
		recent := s.Points[n-tail:]
		f.Recent = round1(float64(transitions(recent)) / float64(len(recent)-1) * 100)
	} else {
		f.Recent = f.Score
	}
	return f, true
}

func transitions(pts []StatusPoint) int {
	changes := 0
	for i := 1; i < len(pts); i++ {
		if pts[i].Status != pts[i-1].Status {
			changes++
		}
	}
	return changes
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
