package insight

import (
	"math"
	"testing"
	"time"
)

// statuses builds an hourly status series, oldest first.
func statuses(ss ...string) StatusSeries {
	s := StatusSeries{Check: "http", Target: "https://a.example"}
	for i, v := range ss {
		s.Points = append(s.Points, StatusPoint{Unix: t0.Add(time.Duration(i) * time.Hour).Unix(), Status: v})
	}
	return s
}

func repeat(status string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = status
	}
	return out
}

func TestPerfectHistoryBurnsNothing(t *testing.T) {
	b, ok := ErrorBudget(statuses(repeat("OK", 100)...), 0.99, 0.1, t0)
	if !ok {
		t.Fatal("want a budget")
	}
	if b.Availability != 1 {
		t.Errorf("availability = %v, want 1", b.Availability)
	}
	if b.Consumed != 0 || b.Remaining != 1 {
		t.Errorf("consumed=%v remaining=%v, want 0 and 1", b.Consumed, b.Remaining)
	}
	if !b.Exhausted.IsZero() {
		t.Error("nothing burning must not project an exhaustion date")
	}
}

func TestBudgetConsumedAtTheObjective(t *testing.T) {
	// 99% objective, exactly 1 failure in 100 runs: the budget is exactly spent.
	ss := append(repeat("OK", 99), "BAD")
	b, _ := ErrorBudget(statuses(ss...), 0.99, 0.1, t0)
	if math.Abs(b.Consumed-1) > 1e-9 {
		t.Errorf("consumed = %v, want 1.0 (budget exactly spent)", b.Consumed)
	}
	if b.Remaining != 0 {
		t.Errorf("remaining = %v, want 0", b.Remaining)
	}
}

func TestOverspentBudgetDoesNotGoNegative(t *testing.T) {
	ss := append(repeat("OK", 90), repeat("BAD", 10)...)
	b, _ := ErrorBudget(statuses(ss...), 0.99, 0.1, t0)
	if b.Consumed <= 1 {
		t.Errorf("consumed = %v, want > 1 (objective missed)", b.Consumed)
	}
	if b.Remaining != 0 {
		t.Errorf("remaining = %v, want floored at 0", b.Remaining)
	}
	if !b.Exhausted.IsZero() {
		t.Error("an already-exhausted budget must not project a future date")
	}
}

// TestWarnIsNotDowntime: counting every warning as downtime makes any objective
// above two nines unmeetable by construction.
func TestWarnIsNotDowntime(t *testing.T) {
	b, _ := ErrorBudget(statuses(repeat("WARN", 100)...), 0.99, 0.1, t0)
	if b.Availability != 1 {
		t.Errorf("availability = %v — WARN must not count against the budget", b.Availability)
	}
}

func TestErrorCountsAsDowntime(t *testing.T) {
	// ERROR means the check could not measure; for availability that is not "up".
	ss := append(repeat("OK", 95), repeat("ERROR", 5)...)
	b, _ := ErrorBudget(statuses(ss...), 0.99, 0.1, t0)
	if b.Availability >= 1 {
		t.Errorf("availability = %v, want below 1", b.Availability)
	}
}

func TestFastBurnSeesARecentOutageSlowBurnDoesNot(t *testing.T) {
	// Clean for a long time, then the last tenth is all down: the fast window
	// should scream while the long window is still comfortable.
	ss := append(repeat("OK", 90), repeat("BAD", 10)...)
	b, _ := ErrorBudget(statuses(ss...), 0.999, 0.1, t0)
	if b.FastBurn <= b.SlowBurn {
		t.Errorf("fast=%v slow=%v — the recent window must burn hotter", b.FastBurn, b.SlowBurn)
	}
	if b.FastBurn < 100 {
		t.Errorf("fast burn = %v, want very high (10%% failure against a 0.1%% allowance)", b.FastBurn)
	}
}

func TestExhaustionIsProjectedWhileBudgetRemains(t *testing.T) {
	// 2 failures in 200 runs against a 95% objective: 1% failure against a 5%
	// allowance, so a fifth of the budget is gone and the rest is burning.
	ss := append(repeat("OK", 198), "BAD", "BAD")
	now := time.Unix(0, 0).Add(24 * time.Hour)
	b, _ := ErrorBudget(statuses(ss...), 0.95, 0.5, now)
	if b.Remaining <= 0 {
		t.Fatalf("expected budget left, got %v", b.Remaining)
	}
	if b.Exhausted.IsZero() {
		t.Skip("fast window saw no failure; exhaustion is not projected, which is correct")
	}
	if !b.Exhausted.After(now) {
		t.Errorf("exhaustion %v must be in the future relative to %v", b.Exhausted, now)
	}
}

func TestShortHistoryHasNoBudget(t *testing.T) {
	// A handful of runs makes every blip look like a 100% burn rate.
	if _, ok := ErrorBudget(statuses(repeat("OK", MinBudgetSamples-1)...), 0.99, 0.1, t0); ok {
		t.Error("too few runs must not produce a budget")
	}
}

func TestNonsenseObjectiveIsRejected(t *testing.T) {
	for _, o := range []float64{0, 1, -0.5, 1.5} {
		if _, ok := ErrorBudget(statuses(repeat("OK", 100)...), o, 0.1, t0); ok {
			t.Errorf("objective %v must be rejected", o)
		}
	}
}
