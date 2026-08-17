package alert

import (
	"path"
	"strings"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// splitKey undoes Key. A target may contain "/" (a URL), so the split is on the
// first separator only — the check name never contains one.
func splitKey(k string) (check, target string) {
	i := strings.Index(k, "/")
	if i < 0 {
		return k, ""
	}
	return k[:i], k[i+1:]
}

// Match returns the first route that covers the event, and whether one did
// (CF-175).
//
// Before this, `alert --provider X` applied to the whole run: either you woke
// the wrong team or you routed nothing. Rules are read in order and the first
// match wins, so the specific ones go on top and a catch-all — a rule with no
// match fields — goes at the bottom.
//
// With no routes configured the caller keeps its flags, so nothing changes for
// anyone who has not asked for this.
//
// labels are the run's global labels, matched exactly: a route asking for
// env=prod fires only when the run carries it.
func Match(routes []engine.AlertRoute, e Event, labels map[string]string) (engine.AlertRoute, bool) {
	for _, r := range routes {
		if !globOK(r.Check, e.Check) || !globOK(r.Target, e.Target) {
			continue
		}
		if !labelsOK(r.Labels, labels) {
			continue
		}
		// A resolve carries no severity — it is the *end* of a problem — so a
		// min_severity filter must not swallow it. Routing a trigger to a team
		// and then resolving it somewhere else would leave an alert open there
		// forever, which is worse than the noise the filter was avoiding.
		if e.Action == "trigger" && !severityOK(r.MinSeverity, e.Severity) {
			continue
		}
		return r, true
	}
	return engine.AlertRoute{}, false
}

func globOK(pattern, s string) bool {
	if pattern == "" {
		return true
	}
	ok, _ := path.Match(pattern, s)
	return ok
}

// labelsOK requires every label the route asks for to be present and equal.
func labelsOK(want, have map[string]string) bool {
	for k, v := range want {
		if have[k] != v {
			return false
		}
	}
	return true
}

func severityOK(min string, s engine.Status) bool {
	if min == "" {
		return true
	}
	threshold, ok := engine.ParseStatus(min)
	if !ok {
		return true // an unparseable threshold filters nothing; validate reports it
	}
	return engine.AtLeast(s, threshold)
}
