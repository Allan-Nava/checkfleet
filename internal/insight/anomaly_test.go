package insight

import (
	"math"
	"testing"
	"time"
)

// hourly builds a series sampled hourly, oldest first.
func hourly(values ...float64) Series {
	s := Series{Check: "http", Target: "https://a.example", Unit: "ms"}
	for i, v := range values {
		s.Points = append(s.Points, Point{Unix: t0.Add(time.Duration(i) * time.Hour).Unix(), Value: v})
	}
	return s
}

func TestSpikeAgainstASteadyBaselineIsAnomalous(t *testing.T) {
	// ~30ms for a week, then 95ms: under any sane static limit, far from its norm.
	s := hourly(30, 31, 29, 30, 32, 28, 31, 95)
	a, dev := Deviating(s, 3)
	if !dev {
		t.Fatalf("want the spike flagged, got %+v", a)
	}
	if a.Baseline < 25 || a.Baseline > 35 {
		t.Errorf("baseline = %v, want ~30", a.Baseline)
	}
	if a.Ratio < 2.5 {
		t.Errorf("ratio = %v, want ~3× the norm", a.Ratio)
	}
}

func TestNormalVariationIsNotAnomalous(t *testing.T) {
	s := hourly(30, 31, 29, 30, 32, 28, 31, 30)
	if a, dev := Deviating(s, 3); dev {
		t.Errorf("ordinary jitter must not be flagged: %+v", a)
	}
}

func TestShortHistoryHasNoBaseline(t *testing.T) {
	// With two readings "the norm" is meaningless and every third one looks odd.
	for n := 0; n <= MinAnomalySamples; n++ {
		vals := make([]float64, n)
		for i := range vals {
			vals[i] = 30
		}
		if _, ok := Deviating(hourly(vals...), 3); ok {
			t.Errorf("%d samples must not yield a baseline", n)
		}
	}
}

func TestPerfectlySteadyMetricDoesNotDivideByZero(t *testing.T) {
	// Zero deviation: a z-score would be infinite, which nobody can act on.
	same := hourly(50, 50, 50, 50, 50, 50, 50, 50)
	a, dev := Deviating(same, 3)
	if dev {
		t.Errorf("an unchanged metric is not anomalous: %+v", a)
	}
	if math.IsInf(a.Z, 0) || math.IsNaN(a.Z) {
		t.Errorf("Z must stay finite, got %v", a.Z)
	}

	moved := hourly(50, 50, 50, 50, 50, 50, 50, 90)
	b, dev := Deviating(moved, 3)
	if !dev {
		t.Error("a move away from a perfectly steady baseline is anomalous")
	}
	if math.IsInf(b.Z, 0) || math.IsNaN(b.Z) {
		t.Errorf("Z must stay finite even with zero deviation, got %v", b.Z)
	}
	if b.Ratio < 1.7 {
		t.Errorf("ratio = %v, want ~1.8 — the usable signal when Z cannot be computed", b.Ratio)
	}
}

func TestBaselineWeightsRecentSamplesMore(t *testing.T) {
	// The EWMA should sit nearer the recent level than a plain mean would.
	s := hourly(10, 10, 10, 10, 80, 80, 80, 80)
	a, _ := Deviating(s, 3)
	plainMean := (10.0*4 + 80.0*3) / 7 // prior excludes the latest sample
	if a.Baseline <= plainMean {
		t.Errorf("EWMA baseline %v should lead the plain mean %v toward the recent level", a.Baseline, plainMean)
	}
}

func TestDropIsAnomalousToo(t *testing.T) {
	// Throughput collapsing matters as much as latency spiking.
	s := hourly(100, 102, 98, 101, 99, 100, 101, 5)
	a, dev := Deviating(s, 3)
	if !dev {
		t.Fatalf("a collapse must be flagged: %+v", a)
	}
	if a.Z >= 0 {
		t.Errorf("Z = %v, want negative for a drop", a.Z)
	}
}
