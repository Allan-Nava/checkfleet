package insight

import (
	"testing"
	"time"
)

func TestCompletedOutagesAreMeasured(t *testing.T) {
	// down for 2 samples (2h), up, down for 1 (1h), up.
	s := statuses("OK", "BAD", "BAD", "OK", "BAD", "OK")
	r := Recoveries(s, t0.Add(6*time.Hour))
	if len(r.Outages) != 2 {
		t.Fatalf("got %d outages, want 2: %v", len(r.Outages), r.Outages)
	}
	if r.Outages[0] != 2*time.Hour || r.Outages[1] != time.Hour {
		t.Errorf("durations = %v, want [2h 1h]", r.Outages)
	}
	if r.Mean != 90*time.Minute {
		t.Errorf("mean = %v, want 1h30m", r.Mean)
	}
	if r.Down {
		t.Error("the target is up at the last sample")
	}
}

func TestOngoingOutageIsMeasuredAgainstNow(t *testing.T) {
	s := statuses("OK", "OK", "BAD", "BAD")
	now := time.Unix(s.Points[3].Unix, 0).Add(47 * time.Minute)
	r := Recoveries(s, now)
	if !r.Down {
		t.Fatal("want Down at the last sample")
	}
	// The outage started at index 2, i.e. 1h before the last sample.
	if want := time.Hour + 47*time.Minute; r.Ongoing != want {
		t.Errorf("ongoing = %v, want %v", r.Ongoing, want)
	}
	if r.Unresolved {
		t.Error("the outage started inside the window, not at its edge")
	}
}

// TestOutageStartingAtTheWindowEdgeIsALowerBound: it began before the history
// we have, so reporting its duration as fact would understate it.
func TestOutageStartingAtTheWindowEdgeIsALowerBound(t *testing.T) {
	s := statuses("BAD", "BAD", "BAD")
	r := Recoveries(s, time.Unix(s.Points[2].Unix, 0))
	if !r.Down || !r.Unresolved {
		t.Errorf("want Down and Unresolved, got %+v", r)
	}
}

func TestWarnIsNotAnOutage(t *testing.T) {
	r := Recoveries(statuses("OK", "WARN", "WARN", "OK"), t0.Add(4*time.Hour))
	if len(r.Outages) != 0 || r.Down {
		t.Errorf("WARN must not count as downtime: %+v", r)
	}
}

func TestErrorIsAnOutage(t *testing.T) {
	r := Recoveries(statuses("OK", "ERROR", "OK"), t0.Add(3*time.Hour))
	if len(r.Outages) != 1 {
		t.Errorf("ERROR must count as downtime: %+v", r)
	}
}

func TestAlwaysUpHasNoRecoveryStats(t *testing.T) {
	r := Recoveries(statuses("OK", "OK", "OK", "OK"), t0.Add(4*time.Hour))
	if len(r.Outages) != 0 || r.Mean != 0 || r.Down {
		t.Errorf("a clean history has nothing to report: %+v", r)
	}
}

func TestPercentilesUseNearestRank(t *testing.T) {
	// Three outages of 1h, 2h, 10h: p90 is the longest. Interpolating would
	// invent precision a three-sample set does not have.
	s := statuses("OK", "BAD", "OK", "BAD", "BAD", "OK",
		"BAD", "BAD", "BAD", "BAD", "BAD", "BAD", "BAD", "BAD", "BAD", "BAD", "OK")
	r := Recoveries(s, t0.Add(20*time.Hour))
	if len(r.Outages) != 3 {
		t.Fatalf("got %d outages, want 3: %v", len(r.Outages), r.Outages)
	}
	if r.P50 != 2*time.Hour {
		t.Errorf("p50 = %v, want 2h", r.P50)
	}
	if r.P90 != 10*time.Hour {
		t.Errorf("p90 = %v, want 10h (the longest of three)", r.P90)
	}
}

func TestEmptySeriesIsSafe(t *testing.T) {
	if r := Recoveries(StatusSeries{}, t0); r.Down || len(r.Outages) != 0 {
		t.Errorf("empty series must yield nothing: %+v", r)
	}
}
