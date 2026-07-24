package main

import (
	"strings"
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/history"
)

func TestDiffFromRecordsAndFormat(t *testing.T) {
	recent := []history.Record{
		{Entries: []history.Entry{{Check: "certs", Target: "a:443", Status: "OK"}, {Check: "http", Target: "x", Status: "BAD"}}},
		{Entries: []history.Entry{{Check: "certs", Target: "a:443", Status: "BAD"}, {Check: "http", Target: "x", Status: "OK"}}},
	}
	changes := diffFromRecords(recent)
	if len(changes) != 2 {
		t.Fatalf("want 2 changes, got %+v", changes)
	}
	out := formatDiff(changes)
	if !strings.Contains(out, "new") || !strings.Contains(out, "resolved") {
		t.Errorf("formatted diff missing kinds:\n%s", out)
	}
	if !strings.Contains(out, "certs") || !strings.Contains(out, "a:443") {
		t.Errorf("formatted diff missing check/target:\n%s", out)
	}
}

func TestFormatDiffEmpty(t *testing.T) {
	if !strings.Contains(formatDiff(nil), "no changes") {
		t.Error("empty diff should say no changes")
	}
}
