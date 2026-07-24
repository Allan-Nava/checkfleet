// Package backlog parses BACKLOG.md into structured items so tooling can keep
// GitHub issues in sync with it. BACKLOG.md is the single source of truth
// (stable CF-n ids); this package never writes it.
package backlog

import (
	"regexp"
	"strings"
)

// Item is one backlog entry (a "CF-n" todo).
type Item struct {
	ID          string // e.g. "CF-4"
	Title       string // issue title, e.g. "CF-4 — Modulo patroni"
	Description string // text after the bold title
	Milestone   string // cleaned section heading, e.g. "M2 — Data layer"
	Done        bool   // true when the checkbox is [x]
}

// itemRe matches a checklist line like:
//
//   - [ ] **CF-4 — Modulo `patroni`**: leader per cluster ...
var itemRe = regexp.MustCompile(`^\s*-\s*\[([ xX])\]\s*\*\*(CF-\d+)\s*—\s*(.*?)\*\*\s*:?\s*(.*)$`)

var headingRe = regexp.MustCompile(`^##\s+(.*)$`)

// Parse extracts the CF-n items from BACKLOG.md content, in file order.
func Parse(md string) []Item {
	var items []Item
	milestone := ""
	for _, line := range strings.Split(md, "\n") {
		if m := headingRe.FindStringSubmatch(line); m != nil {
			milestone = cleanMilestone(m[1])
			continue
		}
		m := itemRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name := stripBackticks(strings.TrimSpace(m[3]))
		items = append(items, Item{
			ID:          m[2],
			Title:       m[2] + " — " + name,
			Description: strings.TrimSpace(m[4]),
			Milestone:   milestone,
			Done:        m[1] == "x" || m[1] == "X",
		})
	}
	return items
}

// cleanMilestone trims a heading to its short title: everything before the
// first " (" (which starts the "(~vX)" / comment tail). "Rilascio" is kept.
func cleanMilestone(h string) string {
	if i := strings.Index(h, " ("); i >= 0 {
		h = h[:i]
	}
	return strings.TrimSpace(h)
}

func stripBackticks(s string) string { return strings.ReplaceAll(s, "`", "") }
