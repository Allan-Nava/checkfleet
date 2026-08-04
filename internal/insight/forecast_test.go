package insight

import (
	"math"
	"testing"
	"time"
)

// day builds a series sampled once a day from t0, with the given values.
func day(t0 time.Time, values ...float64) Series {
	s := Series{Check: "postgres", Target: "db-01", Unit: "%"}
	for i, v := range values {
		s.Points = append(s.Points, Point{Unix: t0.Add(time.Duration(i) * 24 * time.Hour).Unix(), Value: v})
	}
	return s
}

var t0 = time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

func TestETAOnASteadyClimb(t *testing.T) {
	// 80, 82, 84, 86 → +2/day, threshold 90 is 2 days past the last sample.
	s := day(t0, 80, 82, 84, 86)
	now := time.Unix(s.Points[len(s.Points)-1].Unix, 0)
	f, ok := ETAToThreshold(s, 90, now)
	if !ok {
		t.Fatal("want a usable fit")
	}
	if !f.Crosses {
		t.Fatal("a rising series must cross a threshold above it")
	}
	if f.R2 < 0.999 {
		t.Errorf("R2 = %v, want ~1 for a perfectly linear series", f.R2)
	}
	got := f.ETA.Sub(now).Hours() / 24
	if math.Abs(got-2) > 0.01 {
		t.Errorf("ETA is %.3f days out, want 2", got)
	}
}

func TestFlatSeriesGetsNoProjection(t *testing.T) {
	s := day(t0, 50, 50, 50, 50)
	f, ok := ETAToThreshold(s, 90, time.Unix(s.Points[3].Unix, 0))
	if !ok {
		t.Fatal("a flat series is still a usable fit")
	}
	if f.Crosses {
		t.Error("a flat metric must not be projected to cross anything")
	}
	if f.Slope != 0 {
		t.Errorf("slope = %v, want 0", f.Slope)
	}
}

func TestRecedingSeriesGetsNoProjection(t *testing.T) {
	s := day(t0, 86, 84, 82, 80) // falling, threshold above
	f, _ := ETAToThreshold(s, 90, time.Unix(s.Points[3].Unix, 0))
	if f.Crosses {
		t.Error("a metric moving away from the threshold must not get an ETA")
	}
	if f.Slope >= 0 {
		t.Errorf("slope = %v, want negative", f.Slope)
	}
}

func TestFallingTowardALowerThreshold(t *testing.T) {
	// Free disk falling toward a floor: the crossing is downward.
	s := day(t0, 40, 30, 20, 10)
	now := time.Unix(s.Points[3].Unix, 0)
	f, ok := ETAToThreshold(s, 5, now)
	if !ok || !f.Crosses {
		t.Fatalf("want a downward crossing, got %+v", f)
	}
	if got := f.ETA.Sub(now).Hours() / 24; math.Abs(got-0.5) > 0.01 {
		t.Errorf("ETA is %.3f days out, want 0.5", got)
	}
}

func TestTooFewSamplesIsNotAForecast(t *testing.T) {
	// Two points fit a line perfectly and say nothing — the exact shape of a
	// confident-nonsense forecast.
	for _, n := range []int{0, 1, 2, 3} {
		vals := make([]float64, n)
		for i := range vals {
			vals[i] = float64(i * 10)
		}
		if _, ok := ETAToThreshold(day(t0, vals...), 90, t0); ok {
			t.Errorf("%d samples must not produce a forecast", n)
		}
	}
}

func TestNoisySeriesKeepsALowR2(t *testing.T) {
	// Rising on average but erratic: the ETA may exist, R² must say not to trust it.
	s := day(t0, 10, 90, 20, 80, 30, 95)
	f, ok := ETAToThreshold(s, 200, time.Unix(s.Points[5].Unix, 0))
	if !ok {
		t.Fatal("want a fit")
	}
	if f.R2 > 0.5 {
		t.Errorf("R2 = %v, want low for a noisy series", f.R2)
	}
}

func TestSamplesAtOneInstantHaveNoTimeBase(t *testing.T) {
	s := Series{Check: "c", Target: "t"}
	for i := 0; i < 5; i++ {
		s.Points = append(s.Points, Point{Unix: t0.Unix(), Value: float64(i)})
	}
	if _, ok := ETAToThreshold(s, 100, t0); ok {
		t.Error("samples sharing one timestamp cannot be fitted against time")
	}
}

func TestACrossingAlreadyInThePastIsNotReported(t *testing.T) {
	s := day(t0, 80, 84, 88, 92) // already over 90 at the last sample
	f, ok := ETAToThreshold(s, 90, time.Unix(s.Points[3].Unix, 0))
	if !ok {
		t.Fatal("want a fit")
	}
	if f.Crosses {
		t.Error("a past crossing must not be dressed up as a forecast — the current value already says it")
	}
}

func TestDueDistinguishesAPastCrossingFromAFlatMetric(t *testing.T) {
	// Rising toward the threshold, but the samples are old enough that the fit
	// puts the crossing before now. That is not the same as "not trending", and
	// reporting it as such would say "no risk" about the target closest to it.
	s := day(t0, 80, 82, 84, 86)
	late := time.Unix(s.Points[3].Unix, 0).Add(10 * 24 * time.Hour)
	f, ok := ETAToThreshold(s, 90, late)
	if !ok {
		t.Fatal("want a fit")
	}
	if f.Crosses {
		t.Error("a past crossing is not a forecast")
	}
	if !f.Due {
		t.Error("want Due=true: the metric is heading the right way and is overdue")
	}

	flat, _ := ETAToThreshold(day(t0, 50, 50, 50, 50), 90, late)
	if flat.Due {
		t.Error("a flat metric is never Due")
	}
}
