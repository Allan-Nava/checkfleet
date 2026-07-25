package engine

import (
	"path"
	"strconv"
	"strings"
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
		if !inDailyWindow(now, w.Daily, w.Weekdays) {
			continue
		}
		return w, true
	}
	return MaintenanceWindow{}, false
}

// inDailyWindow reports whether now falls in a recurring window: the local clock
// time within "HH:MM-HH:MM" (wrapping past midnight) and, if weekdays is set, on
// one of those days. An empty daily range means "not daily-restricted" (true).
func inDailyWindow(now time.Time, daily string, weekdays []string) bool {
	if strings.TrimSpace(daily) == "" {
		return true
	}
	start, end, ok := parseDaily(daily)
	if !ok {
		return false // malformed range never matches (fail safe: don't mute)
	}
	if len(weekdays) > 0 && !weekdayMatch(now.Weekday(), weekdays) {
		return false
	}
	m := now.Hour()*60 + now.Minute()
	if start <= end {
		return m >= start && m <= end
	}
	return m >= start || m <= end // wraps past midnight
}

// parseDaily parses "HH:MM-HH:MM" into start/end minutes-of-day.
func parseDaily(s string) (start, end int, ok bool) {
	a, b, found := strings.Cut(s, "-")
	if !found {
		return 0, 0, false
	}
	start, ok = parseHHMM(a)
	if !ok {
		return 0, 0, false
	}
	end, ok = parseHHMM(b)
	return start, end, ok
}

func parseHHMM(s string) (int, bool) {
	h, m, found := strings.Cut(strings.TrimSpace(s), ":")
	if !found {
		return 0, false
	}
	hh, err1 := strconv.Atoi(h)
	mm, err2 := strconv.Atoi(m)
	if err1 != nil || err2 != nil || hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return 0, false
	}
	return hh*60 + mm, true
}

func weekdayMatch(d time.Weekday, weekdays []string) bool {
	name := strings.ToLower(d.String()) // e.g. "saturday"
	for _, w := range weekdays {
		w = strings.ToLower(strings.TrimSpace(w))
		if w == name || (len(w) >= 3 && strings.HasPrefix(name, w)) {
			return true
		}
	}
	return false
}
