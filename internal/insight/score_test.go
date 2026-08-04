package insight

import (
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

func fnd(check, target string, s engine.Status) engine.Finding {
	return engine.Finding{Check: check, Target: target, Status: s, Message: "x"}
}

func TestAllGreenScores100(t *testing.T) {
	s := FleetScore([]engine.Finding{
		fnd("http", "a", engine.OK), fnd("certs", "b", engine.OK),
	}, nil)
	if s.Value != 100 {
		t.Errorf("score = %v, want 100", s.Value)
	}
	if s.Penalty != 0 {
		t.Errorf("penalty = %v, want 0", s.Penalty)
	}
}

func TestEmptyFleetScores100(t *testing.T) {
	if s := FleetScore(nil, nil); s.Value != 100 {
		t.Errorf("score = %v, want 100 for nothing to check", s.Value)
	}
}

func TestEverythingBadAndFlappingScoresZero(t *testing.T) {
	unstable := map[string]bool{"http\x00a": true, "http\x00b": true}
	s := FleetScore([]engine.Finding{
		fnd("http", "a", engine.BAD), fnd("http", "b", engine.BAD),
	}, unstable)
	if s.Value != 0 {
		t.Errorf("score = %v, want 0 for the worst possible fleet", s.Value)
	}
}

func TestBadCostsMoreThanWarn(t *testing.T) {
	warn := FleetScore([]engine.Finding{fnd("http", "a", engine.WARN)}, nil)
	bad := FleetScore([]engine.Finding{fnd("http", "a", engine.BAD)}, nil)
	if bad.Value >= warn.Value {
		t.Errorf("BAD (%v) must score worse than WARN (%v)", bad.Value, warn.Value)
	}
}

// TestErrorCostsSlightlyLessThanBad: "we could not measure" is bad news about
// the probe and only maybe about the target. Pricing them identically lets a
// network blip during a run look exactly like an outage.
func TestErrorCostsSlightlyLessThanBad(t *testing.T) {
	bad := FleetScore([]engine.Finding{fnd("http", "a", engine.BAD)}, nil)
	erp := FleetScore([]engine.Finding{fnd("http", "a", engine.ERROR)}, nil)
	if !(erp.Value > bad.Value) {
		t.Errorf("ERROR (%v) should score above BAD (%v)", erp.Value, bad.Value)
	}
	if erp.Value >= 100 {
		t.Errorf("ERROR (%v) must still cost something", erp.Value)
	}
}

func TestFlappingCostsOnTopOfStatus(t *testing.T) {
	stable := FleetScore([]engine.Finding{fnd("http", "a", engine.WARN)}, nil)
	flapping := FleetScore([]engine.Finding{fnd("http", "a", engine.WARN)},
		map[string]bool{"http\x00a": true})
	if flapping.Value >= stable.Value {
		t.Errorf("a flapping WARN (%v) must score worse than a stable one (%v)", flapping.Value, stable.Value)
	}
	if flapping.Unstable != 1 {
		t.Errorf("unstable = %d, want 1", flapping.Unstable)
	}
}

// TestScoreIsComparableAcrossFleetSizes: an index that drifts with the number of
// targets cannot be watched over time, which is the only reason to have one.
func TestScoreIsComparableAcrossFleetSizes(t *testing.T) {
	small := FleetScore([]engine.Finding{
		fnd("http", "a", engine.BAD), fnd("http", "b", engine.OK),
	}, nil)
	big := []engine.Finding{}
	for i := 0; i < 50; i++ {
		big = append(big, fnd("http", string(rune('a'+i%26))+string(rune('0'+i/26)), engine.BAD))
		big = append(big, fnd("certs", string(rune('a'+i%26))+string(rune('0'+i/26)), engine.OK))
	}
	bigScore := FleetScore(big, nil)
	if diff := small.Value - bigScore.Value; diff < -0.5 || diff > 0.5 {
		t.Errorf("same 50%% BAD ratio scored %v (2 findings) vs %v (100 findings)", small.Value, bigScore.Value)
	}
}

func TestModuleBreakdownPointsAtTheWorst(t *testing.T) {
	fs := []engine.Finding{
		fnd("http", "a", engine.OK), fnd("http", "b", engine.OK),
		fnd("certs", "c", engine.BAD), fnd("certs", "d", engine.BAD),
	}
	scores := ModuleScores(fs, nil)
	order := SortedModules(scores)
	if order[0] != "certs" {
		t.Errorf("worst module first: got %v", order)
	}
	if scores["http"].Value != 100 {
		t.Errorf("http = %v, want 100", scores["http"].Value)
	}
}

func TestUnstableKeysCountsTransitions(t *testing.T) {
	flapping := statuses("OK", "BAD", "OK", "BAD", "OK")
	steady := StatusSeries{Check: "certs", Target: "b", Points: statuses("OK", "OK", "OK").Points}
	keys := UnstableKeys([]StatusSeries{flapping, steady}, 3)
	if !keys[flapping.Key()] {
		t.Error("the oscillating target should be flagged unstable")
	}
	if keys[steady.Key()] {
		t.Error("a steady target must not be flagged")
	}
}

func TestSortedModulesIsStable(t *testing.T) {
	scores := map[string]Score{"b": {Value: 50}, "a": {Value: 50}, "c": {Value: 10}}
	for i := 0; i < 5; i++ {
		got := SortedModules(scores)
		if got[0] != "c" || got[1] != "a" || got[2] != "b" {
			t.Fatalf("unstable order: %v", got)
		}
	}
}
