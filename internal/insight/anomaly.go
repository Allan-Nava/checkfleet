package insight

import "math"

// Anomaly reports how far the latest sample sits from the metric's *own*
// recent normal (CF-122). A static threshold cannot see this: a latency of
// 90ms is fine against a 500ms limit and alarming if this target has sat at
// 30ms for a week. The baseline is an exponentially weighted moving average, so
// recent behaviour counts more than last month's.
type Anomaly struct {
	// Baseline is the EWMA of the samples before the latest one.
	Baseline float64
	// Deviation is the EWMA standard deviation around that baseline.
	Deviation float64
	// Latest is the newest sample.
	Latest float64
	// Z is how many deviations the latest sample sits from the baseline, signed.
	Z float64
	// Ratio is Latest/Baseline, for the "3× the norm" phrasing that reads better
	// than a z-score. Zero when the baseline is zero.
	Ratio float64
	// Samples is how many points fed the baseline (the latest is excluded).
	Samples int
}

// MinAnomalySamples is the shortest history worth calling a baseline. Below
// this, "the norm" is one or two readings and every third one looks anomalous.
const MinAnomalySamples = 6

// defaultAlpha weights the EWMA. 0.3 gives the last handful of runs most of the
// say while still remembering a week of hourly checks.
const defaultAlpha = 0.3

// Deviating reports whether the latest sample of s deviates from its own recent
// baseline by more than z standard deviations.
//
// ok=false when the series is too short to have a normal. A metric that has
// never moved has zero deviation: rather than dividing by zero and calling
// every change infinitely anomalous, that case is reported as not deviating
// unless the value actually changed, and then with a Z of ±inf avoided by
// falling back to the ratio.
func Deviating(s Series, z float64) (Anomaly, bool) {
	n := len(s.Points)
	if n < MinAnomalySamples+1 {
		return Anomaly{Samples: max(0, n-1)}, false
	}
	prior := s.Points[:n-1]
	latest := s.Points[n-1].Value

	mean, variance := ewmaStats(prior, defaultAlpha)
	a := Anomaly{
		Baseline:  mean,
		Deviation: math.Sqrt(variance),
		Latest:    latest,
		Samples:   len(prior),
	}
	if mean != 0 {
		a.Ratio = latest / mean
	}
	if a.Deviation == 0 {
		// A perfectly steady metric. Any move is "infinitely" anomalous by
		// z-score, which is a number nobody can act on; leave Z at zero and let
		// the caller use Ratio, which stays meaningful.
		return a, latest != mean
	}
	a.Z = (latest - mean) / a.Deviation
	return a, math.Abs(a.Z) >= z
}

// ewmaStats returns the exponentially weighted mean and variance of pts.
func ewmaStats(pts []Point, alpha float64) (mean, variance float64) {
	mean = pts[0].Value
	for _, p := range pts[1:] {
		diff := p.Value - mean
		inc := alpha * diff
		mean += inc
		variance = (1 - alpha) * (variance + diff*inc)
	}
	return mean, variance
}
