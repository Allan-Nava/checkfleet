package insight

import (
	"math"
	"sort"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// Score is a single 0–100 index for a fleet or a module (CF-127). It exists so
// there is one number to watch instead of a wall of rows — not to replace the
// rows, which stay the only place the actual problem is written down.
type Score struct {
	// Value is the index, 0 (everything broken) to 100 (all green and steady).
	Value float64
	// Findings is how many findings fed it.
	Findings int
	// Penalty is the total weight subtracted, before scaling.
	Penalty float64
	// Unstable counts targets whose recent history flapped, which are penalised
	// beyond their current status.
	Unstable int
}

// StatusWeights is how much each status costs the score. ERROR costs slightly
// less than BAD on purpose: "we could not measure" is bad news about the probe
// and only maybe about the target, and pricing it identically would let a
// network blip during a run look exactly like an outage.
var StatusWeights = map[engine.Status]float64{
	engine.OK:    0,
	engine.WARN:  1,
	engine.BAD:   5,
	engine.ERROR: 4,
}

// InstabilityWeight is what a flapping target costs on top of its status. A
// target that oscillates is worse than a stable BAD you have already triaged:
// it wakes people up repeatedly and hides real transitions in the noise.
const InstabilityWeight = 3

// FleetScore reduces a run to a single index, penalising each finding by
// severity and adding a surcharge for targets that flapped in the recent
// history. Pass nil history for a status-only score.
//
// The weights are exported and documented because a score whose arithmetic is
// hidden is a number nobody trusts twice.
func FleetScore(findings []engine.Finding, unstableKeys map[string]bool) Score {
	s := Score{Findings: len(findings)}
	if len(findings) == 0 {
		s.Value = 100
		return s
	}
	for _, f := range findings {
		s.Penalty += StatusWeights[f.Status]
		if unstableKeys[f.Check+"\x00"+f.Target] {
			s.Penalty += InstabilityWeight
			s.Unstable++
		}
	}
	// Normalise against the worst this fleet could score: every finding BAD and
	// flapping. That keeps the index comparable across fleets of different
	// sizes, which is the whole point of having one number.
	worst := float64(len(findings)) * (StatusWeights[engine.BAD] + InstabilityWeight)
	s.Value = math.Round((1-s.Penalty/worst)*1000) / 10
	if s.Value < 0 {
		s.Value = 0
	}
	return s
}

// UnstableKeys returns the check+target keys that changed status at least
// minChanges times in the history, in the shape FleetScore wants.
func UnstableKeys(series []StatusSeries, minChanges int) map[string]bool {
	out := map[string]bool{}
	for _, s := range series {
		changes := 0
		for i := 1; i < len(s.Points); i++ {
			if s.Points[i].Status != s.Points[i-1].Status {
				changes++
			}
		}
		if changes >= minChanges {
			out[s.Key()] = true
		}
	}
	return out
}

// ModuleScores breaks the fleet index down per module, worst first, so the one
// number has somewhere to point.
func ModuleScores(findings []engine.Finding, unstableKeys map[string]bool) map[string]Score {
	byModule := map[string][]engine.Finding{}
	for _, f := range findings {
		byModule[f.Check] = append(byModule[f.Check], f)
	}
	out := make(map[string]Score, len(byModule))
	for name, fs := range byModule {
		out[name] = FleetScore(fs, unstableKeys)
	}
	return out
}

// SortedModules returns module names ordered by ascending score (worst first),
// then by name, so the output is stable.
func SortedModules(scores map[string]Score) []string {
	names := make([]string, 0, len(scores))
	for n := range scores {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		a, b := scores[names[i]], scores[names[j]]
		if a.Value != b.Value {
			return a.Value < b.Value
		}
		return names[i] < names[j]
	})
	return names
}
