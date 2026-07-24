package engine

import (
	"path"
	"time"
)

// ApplyMaintenance drops or downgrades findings that fall inside an active
// maintenance window at time now, so planned work doesn't page. The first
// matching window wins. Action "mute" (default) removes the finding; "warn"
// caps BAD/ERROR at WARN and annotates the message with "[maintenance]".
func ApplyMaintenance(findings []Finding, windows []MaintenanceWindow, now time.Time) []Finding {
	if len(windows) == 0 {
		return findings
	}
	out := findings[:0:0] // new slice, keep input untouched
	for _, f := range findings {
		w, active := activeWindow(f, windows, now)
		if !active {
			out = append(out, f)
			continue
		}
		if w.Action == "warn" {
			if f.Status == BAD || f.Status == ERROR {
				f.Status = WARN
			}
			f.Message += " [maintenance]"
			out = append(out, f)
		}
		// "mute" (default): drop the finding.
	}
	return out
}

// activeWindow returns the first window matching the finding and active at now.
func activeWindow(f Finding, windows []MaintenanceWindow, now time.Time) (MaintenanceWindow, bool) {
	for _, w := range windows {
		if w.Check != "" {
			if ok, _ := path.Match(w.Check, f.Check); !ok {
				continue
			}
		}
		if w.Target != "" {
			if ok, _ := path.Match(w.Target, f.Target); !ok {
				continue
			}
		}
		if w.From != "" {
			if from, err := time.Parse(time.RFC3339, w.From); err != nil || now.Before(from) {
				continue
			}
		}
		if w.To != "" {
			if to, err := time.Parse(time.RFC3339, w.To); err != nil || now.After(to) {
				continue
			}
		}
		return w, true
	}
	return MaintenanceWindow{}, false
}
