package main

import (
	"fmt"
	"strings"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/history"
)

// recordMap turns a history record into a key→status map, keyed by
// "check\ttarget" so the check and target can be split back out for display.
func recordMap(r history.Record) map[string]engine.Status {
	m := make(map[string]engine.Status, len(r.Entries))
	for _, e := range r.Entries {
		m[e.Check+"\t"+e.Target] = engine.Status(e.Status)
	}
	return m
}

// diffFromRecords compares the last two history records (previous vs current).
func diffFromRecords(recent []history.Record) []engine.Change {
	if len(recent) == 0 {
		return nil
	}
	curr := recordMap(recent[len(recent)-1])
	var prev map[string]engine.Status
	if len(recent) >= 2 {
		prev = recordMap(recent[len(recent)-2])
	}
	return engine.DiffStatus(prev, curr)
}

var changeSymbol = map[engine.ChangeKind]string{
	engine.ChangeNew:      "+",
	engine.ChangeResolved: "-",
	engine.ChangeWorsened: "!",
	engine.ChangeImproved: "~",
}

// formatDiff renders the changes as text (empty → a "no changes" note).
func formatDiff(changes []engine.Change) string {
	if len(changes) == 0 {
		return "no changes since the previous run\n"
	}
	var b strings.Builder
	b.WriteString("changes since the previous run:\n")
	for _, c := range changes {
		check, target, _ := strings.Cut(c.Key, "\t")
		fmt.Fprintf(&b, "  %s %-8s %-9s %-40s %s→%s\n",
			changeSymbol[c.Kind], c.Kind, check, target, c.From, c.To)
	}
	return b.String()
}
