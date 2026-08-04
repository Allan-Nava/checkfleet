package insight

import (
	"math"
	"time"
)

// Forecast projects when a metric will cross a threshold, from the slope of its
// recent history (CF-121). It generalises the ETA that today only certificates
// have: "disk 82% → crosses 90% in ~2.5 days" is the same question as "this cert
// expires in 12 days", asked of any metric checkfleet already records.
type Forecast struct {
	// Slope is the fitted rate of change, in metric units per second.
	Slope float64
	// R2 is the coefficient of determination, 0..1: how well a straight line
	// explains the samples. Low R2 means the projection below is noise dressed
	// as a number, so callers should show it or suppress the ETA.
	R2 float64
	// ETA is when the fitted line reaches the threshold. Only meaningful when
	// Crosses is true.
	ETA time.Time
	// Crosses reports whether the trend reaches the threshold in the future at
	// all: a flat or receding metric never does.
	Crosses bool
	// Due reports that the fit puts the crossing in the *past*: the metric is
	// heading the right way and should already be over the line. That is a
	// different statement from "not trending", and conflating the two reads as
	// "no risk" on a target that is already there.
	Due bool
	// Samples is how many points the fit used.
	Samples int
}

// MinForecastSamples is the smallest series worth fitting. Two points always fit
// a line perfectly (R2 = 1) and tell you nothing about the trend, which is
// exactly how a forecast feature produces confident nonsense.
const MinForecastSamples = 4

// ETAToThreshold fits a least-squares line to the series and projects when it
// crosses threshold, coming from whichever side the latest sample is on.
//
// Returns ok=false when the series is too short to fit, when every sample shares
// one timestamp (no time base), or when the metric is not moving toward the
// threshold. It never invents a crossing for a flat line: no projection is a
// better answer than a projection nobody should act on.
func ETAToThreshold(s Series, threshold float64, now time.Time) (Forecast, bool) {
	n := len(s.Points)
	if n < MinForecastSamples {
		return Forecast{Samples: n}, false
	}
	slope, intercept, r2, ok := fitLine(s.Points)
	if !ok {
		return Forecast{Samples: n}, false
	}
	f := Forecast{Slope: slope, R2: r2, Samples: n}

	latest := s.Points[n-1].Value
	// Moving away from (or parallel to) the threshold: no crossing to report.
	rising := threshold > latest
	if slope == 0 || (rising && slope <= 0) || (!rising && slope >= 0) {
		return f, true
	}
	// Solve intercept + slope*t = threshold for t (seconds since the epoch used
	// by fitLine, which is the first sample's timestamp).
	t := (threshold - intercept) / slope
	at := time.Unix(s.Points[0].Unix, 0).Add(time.Duration(t * float64(time.Second)))
	if !at.After(now) {
		// The fit says it should already have crossed. A past ETA is not a
		// forecast — the current value is the honest source — but the caller must
		// still be able to tell this apart from a flat metric.
		f.Due = true
		return f, true
	}
	f.ETA, f.Crosses = at, true
	return f, true
}

// fitLine is ordinary least squares over (seconds-since-first-point, value),
// returning slope, intercept and R². ok=false when the x values have no spread.
func fitLine(pts []Point) (slope, intercept, r2 float64, ok bool) {
	n := float64(len(pts))
	base := pts[0].Unix
	var sx, sy float64
	for _, p := range pts {
		sx += float64(p.Unix - base)
		sy += p.Value
	}
	mx, my := sx/n, sy/n

	var sxy, sxx, syy float64
	for _, p := range pts {
		dx := float64(p.Unix-base) - mx
		dy := p.Value - my
		sxy += dx * dy
		sxx += dx * dx
		syy += dy * dy
	}
	if sxx == 0 {
		return 0, 0, 0, false // every sample at the same instant
	}
	slope = sxy / sxx
	intercept = my - slope*mx
	if syy == 0 {
		// A perfectly flat series: the line explains it exactly, and the slope is
		// zero. R²=1 is correct and harmless here because Crosses stays false.
		return slope, intercept, 1, true
	}
	r2 = (sxy * sxy) / (sxx * syy)
	if math.IsNaN(r2) {
		return slope, intercept, 0, true
	}
	return slope, intercept, math.Min(r2, 1), true
}
