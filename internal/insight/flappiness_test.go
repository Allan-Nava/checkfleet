package insight

import "testing"

func alternating(n int) []string {
	out := make([]string, n)
	for i := range out {
		if i%2 == 0 {
			out[i] = "OK"
		} else {
			out[i] = "BAD"
		}
	}
	return out
}

func TestSteadyTargetScoresZero(t *testing.T) {
	f, ok := Flapping(statuses(repeat("OK", 20)...))
	if !ok {
		t.Fatal("want a score")
	}
	if f.Score != 0 || f.Changes != 0 {
		t.Errorf("a target that never moved should score 0: %+v", f)
	}
	if f.Level() != "" {
		t.Errorf("level = %q, want empty (no badge)", f.Level())
	}
}

func TestAlternatingEveryRunScores100(t *testing.T) {
	f, _ := Flapping(statuses(alternating(20)...))
	if f.Score != 100 {
		t.Errorf("score = %v, want 100 for a change at every run", f.Score)
	}
	if f.Level() != "high" {
		t.Errorf("level = %q, want high", f.Level())
	}
}

// TestTheSameCountScoresDifferentlyOverDifferentWindows is the whole point of
// CF-171: CF-32's raw count cannot tell these two apart.
func TestTheSameCountScoresDifferentlyOverDifferentWindows(t *testing.T) {
	// 4 changes in 10 runs vs 4 changes in 60.
	short := statuses(append(alternating(5), repeat("OK", 5)...)...)
	long := statuses(append(alternating(5), repeat("OK", 55)...)...)

	s, _ := Flapping(short)
	l, _ := Flapping(long)
	if s.Changes != l.Changes {
		t.Fatalf("setup wrong: %d vs %d changes", s.Changes, l.Changes)
	}
	if !(s.Score > l.Score) {
		t.Errorf("the same %d changes should score higher over 10 runs (%v) than over 60 (%v)",
			s.Changes, s.Score, l.Score)
	}
}

// TestRecentSeesATargetThatSettled — "was flapping, steady since" must not be
// averaged away into a mid score with no direction.
func TestRecentSeesATargetThatSettled(t *testing.T) {
	f, _ := Flapping(statuses(append(alternating(20), repeat("OK", 20)...)...))
	if f.Score == 0 {
		t.Fatal("the window still contains the oscillation")
	}
	if f.Recent >= f.Score {
		t.Errorf("recent (%v) should be below the window score (%v) for a settled target", f.Recent, f.Score)
	}
}

func TestRecentSeesATargetThatJustStarted(t *testing.T) {
	f, _ := Flapping(statuses(append(repeat("OK", 20), alternating(20)...)...))
	if f.Recent <= f.Score {
		t.Errorf("recent (%v) should exceed the window score (%v) for a target that just started", f.Recent, f.Score)
	}
}

func TestShortHistoryIsNotScored(t *testing.T) {
	// Three samples make one blip read as 50% flappiness: a number, not a signal.
	for n := 0; n < MinFlapSamples; n++ {
		if _, ok := Flapping(statuses(alternating(n)...)); ok {
			t.Errorf("%d samples must not produce a score", n)
		}
	}
}

func TestLevelBuckets(t *testing.T) {
	cases := map[float64]string{0: "", 5: "low", 25: "medium", 60: "high", 100: "high"}
	for score, want := range cases {
		if got := (Flappiness{Score: score}).Level(); got != want {
			t.Errorf("Level(%v) = %q, want %q", score, got, want)
		}
	}
}

func TestWarnToBadCountsAsATransition(t *testing.T) {
	// Oscillating between two problem states is still instability, even though
	// the target is never OK.
	f, _ := Flapping(statuses("WARN", "BAD", "WARN", "BAD", "WARN", "BAD"))
	if f.Changes != 5 {
		t.Errorf("changes = %d, want 5", f.Changes)
	}
}
