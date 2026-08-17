package engine

import (
	"fmt"
	"sort"
	"time"
)

// ValidateAlertRoutes reports rules that would silently misroute (CF-175).
//
// A routing mistake is the kind that shows up at 3am on the wrong phone, so the
// checks here are deliberately strict about the things that produce silence: an
// unknown provider, a missing key, and a rule placed after a catch-all where it
// can never be reached.
func ValidateAlertRoutes(routes []AlertRoute) []string {
	var problems []string
	catchAll := -1
	for i, r := range routes {
		switch r.Provider {
		case "pagerduty", "opsgenie":
			if r.KeyEnv == "" {
				problems = append(problems, fmt.Sprintf(
					"alert_routes[%d]: %s needs key_env (the env var holding the key, never the key itself)", i, r.Provider))
			}
		case "sns":
			if r.SNSTopicARN == "" {
				problems = append(problems, fmt.Sprintf("alert_routes[%d]: sns needs sns_topic_arn", i))
			}
		case "":
			problems = append(problems, fmt.Sprintf("alert_routes[%d]: provider is required", i))
		default:
			problems = append(problems, fmt.Sprintf(
				"alert_routes[%d]: unknown provider %q (pagerduty|opsgenie|sns)", i, r.Provider))
		}
		if r.RenotifyAfter != "" {
			if _, err := time.ParseDuration(r.RenotifyAfter); err != nil {
				problems = append(problems, fmt.Sprintf(
					"alert_routes[%d]: renotify_after %q is not a duration (e.g. 4h, 30m)", i, r.RenotifyAfter))
			}
		}
		if r.MinSeverity != "" {
			if _, ok := ParseStatus(r.MinSeverity); !ok {
				problems = append(problems, fmt.Sprintf(
					"alert_routes[%d]: min_severity %q is not valid (warn|bad|error)", i, r.MinSeverity))
			}
		}
		// A rule with no match fields catches everything, so anything after it
		// is dead. Reported because the symptom — alerts going to the wrong
		// place — looks like the routing not working at all.
		isCatchAll := r.Check == "" && r.Target == "" && r.MinSeverity == "" && len(r.Labels) == 0
		if catchAll >= 0 {
			problems = append(problems, fmt.Sprintf(
				"alert_routes[%d]: unreachable — alert_routes[%d] matches everything before it", i, catchAll))
		}
		if isCatchAll && catchAll < 0 {
			catchAll = i
		}
	}
	return problems
}

// ValidateHistoryRetention reports durations that would silently do nothing
// (CF-177). A typo in "30d" — which Go does not parse — would otherwise leave
// the file growing while the config claims a limit.
func ValidateHistoryRetention(h HistoryRetention) []string {
	var problems []string
	for name, v := range map[string]string{"max_age": h.MaxAge, "downsample_after": h.DownsampleAfter} {
		if v == "" {
			continue
		}
		if _, err := time.ParseDuration(v); err != nil {
			problems = append(problems, fmt.Sprintf(
				"history_retention.%s %q is not a duration — Go has no day unit, use hours (720h for 30 days)", name, v))
		}
	}
	if h.MaxRuns < 0 {
		problems = append(problems, "history_retention.max_runs cannot be negative")
	}
	sort.Strings(problems)
	return problems
}
