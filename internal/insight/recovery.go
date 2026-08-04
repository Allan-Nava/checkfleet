package insight

import (
	"sort"
	"time"
)

// Recovery is how long a target takes to come back, and how long it has been
// down this time (CF-126). "Down for 47m, usually recovers in ~8m" is a
// different call from "down for 47m, usually takes 2h" — the number that turns
// a red row into a decision is the comparison, not the duration.
type Recovery struct {
	// Outages are the completed down→up episodes found in the window.
	Outages []time.Duration
	// Mean, P50 and P90 of those episodes. Zero when there were none.
	Mean, P50, P90 time.Duration
	// Ongoing is how long the current outage has lasted, when the target is
	// down at the last sample. Zero when it is up.
	Ongoing time.Duration
	// Down reports whether the target is down at the newest sample.
	Down bool
	// Unresolved marks an ongoing outage that started at the very first sample:
	// it began before the window, so Ongoing is a lower bound, not a duration.
	Unresolved bool
}

// Recoveries summarises the down→up episodes in a status series. An episode
// runs from the first down sample to the next up sample; its duration is the
// gap between those timestamps.
//
// A run that is down is BAD or ERROR, the same rule the error budget uses:
// WARN is a warning, not an outage.
func Recoveries(s StatusSeries, now time.Time) Recovery {
	var r Recovery
	if len(s.Points) == 0 {
		return r
	}
	var start int64
	inOutage := false
	startedAtWindowEdge := false

	for i, p := range s.Points {
		switch {
		case isDown(p.Status) && !inOutage:
			inOutage, start = true, p.Unix
			startedAtWindowEdge = i == 0
		case !isDown(p.Status) && inOutage:
			inOutage = false
			if d := time.Duration(p.Unix-start) * time.Second; d > 0 {
				r.Outages = append(r.Outages, d)
			}
		}
	}
	if inOutage {
		r.Down = true
		r.Unresolved = startedAtWindowEdge
		if d := now.Sub(time.Unix(start, 0)); d > 0 {
			r.Ongoing = d
		}
	}
	if len(r.Outages) > 0 {
		r.Mean, r.P50, r.P90 = summarise(r.Outages)
	}
	return r
}

// summarise returns mean, p50 and p90 of the durations. Percentiles use the
// nearest-rank method: with three outages, "p90" is the longest one — an honest
// answer for a small sample, where interpolation would invent precision.
func summarise(in []time.Duration) (mean, p50, p90 time.Duration) {
	d := append([]time.Duration(nil), in...)
	sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
	var total time.Duration
	for _, v := range d {
		total += v
	}
	mean = total / time.Duration(len(d))
	return mean, percentile(d, 0.50), percentile(d, 0.90)
}

// percentile takes the nearest-rank value from a sorted slice.
func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	rank := int(float64(len(sorted))*p+0.9999) - 1
	if rank < 0 {
		rank = 0
	}
	if rank >= len(sorted) {
		rank = len(sorted) - 1
	}
	return sorted[rank]
}
