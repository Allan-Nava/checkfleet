// Package insight turns the history checkfleet already persists into signal
// (M30). It is domain analysis, not a dashboard: graphing and alerting stay
// delegated to the tools that do them well.
//
// Everything here is a pure function over history records — no I/O, no clock of
// its own (the caller passes `now`), no dependencies beyond the standard
// library. Statistics are written by hand on purpose: the whole point of the
// zero-dep rule is that a monitoring binary you run everywhere should not carry
// a numerical stack it uses for two regressions.
//
// The CLI and the desktop both call into here, so an insight means the same
// thing in both.
package insight

import (
	"sort"

	"github.com/Allan-Nava/checkfleet/internal/history"
)

// Point is one metric sample.
type Point struct {
	Unix  int64
	Value float64
}

// Series is the metric history of a single check/target pair, oldest first.
// Only findings that carried a Value produce points: most modules report a
// status and no scalar, and those have no series at all.
type Series struct {
	Check  string
	Target string
	Unit   string
	Points []Point
}

// Key is the finding identity — the same check+target pair used everywhere else
// for deduplication (issues, alerts, baselines).
func (s Series) Key() string { return s.Check + "\x00" + s.Target }

// StatusPoint is one status observation, for the analyses that work on the
// status sequence rather than on a scalar.
type StatusPoint struct {
	Unix   int64
	Status string
}

// StatusSeries is the status history of one check/target pair, oldest first.
type StatusSeries struct {
	Check  string
	Target string
	Points []StatusPoint
}

// Key mirrors Series.Key.
func (s StatusSeries) Key() string { return s.Check + "\x00" + s.Target }

// SeriesFrom groups records into one metric series per check/target, sorted by
// key so callers get a stable order. Records need not be sorted: points are
// ordered by timestamp here.
func SeriesFrom(records []history.Record) []Series {
	byKey := map[string]*Series{}
	for _, r := range records {
		for _, e := range r.Entries {
			if e.Value == nil {
				continue
			}
			k := e.Check + "\x00" + e.Target
			s := byKey[k]
			if s == nil {
				s = &Series{Check: e.Check, Target: e.Target, Unit: e.Unit}
				byKey[k] = s
			}
			if s.Unit == "" {
				s.Unit = e.Unit
			}
			s.Points = append(s.Points, Point{Unix: r.Unix, Value: *e.Value})
		}
	}
	return finish(byKey)
}

// StatusSeriesFrom groups records into one status series per check/target.
func StatusSeriesFrom(records []history.Record) []StatusSeries {
	byKey := map[string]*StatusSeries{}
	for _, r := range records {
		for _, e := range r.Entries {
			k := e.Check + "\x00" + e.Target
			s := byKey[k]
			if s == nil {
				s = &StatusSeries{Check: e.Check, Target: e.Target}
				byKey[k] = s
			}
			s.Points = append(s.Points, StatusPoint{Unix: r.Unix, Status: e.Status})
		}
	}
	out := make([]StatusSeries, 0, len(byKey))
	for _, s := range byKey {
		sort.SliceStable(s.Points, func(i, j int) bool { return s.Points[i].Unix < s.Points[j].Unix })
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key() < out[j].Key() })
	return out
}

func finish(byKey map[string]*Series) []Series {
	out := make([]Series, 0, len(byKey))
	for _, s := range byKey {
		sort.SliceStable(s.Points, func(i, j int) bool { return s.Points[i].Unix < s.Points[j].Unix })
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key() < out[j].Key() })
	return out
}
