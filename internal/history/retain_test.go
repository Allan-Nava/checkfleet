package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

var now = time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)

// hourly builds n records, one per hour, ending at `now`.
func hourly(n int) []Record {
	out := make([]Record, 0, n)
	for i := n - 1; i >= 0; i-- {
		out = append(out, Record{
			Unix:    now.Add(-time.Duration(i) * time.Hour).Unix(),
			Schema:  SchemaVersion,
			Entries: []Entry{{Check: "http", Target: "a", Status: "OK"}},
		})
	}
	return out
}

func TestNoPolicyKeepsEverything(t *testing.T) {
	in := hourly(100)
	if got := (RetentionPolicy{}).Apply(in, now); len(got) != 100 {
		t.Errorf("kept %d of 100 with no policy", len(got))
	}
}

func TestMaxRunsKeepsTheNewest(t *testing.T) {
	got := RetentionPolicy{MaxRuns: 10}.Apply(hourly(100), now)
	if len(got) != 10 {
		t.Fatalf("kept %d, want 10", len(got))
	}
	if got[len(got)-1].Unix != now.Unix() {
		t.Error("the newest record must survive")
	}
}

func TestMaxAgeDropsTheOld(t *testing.T) {
	got := RetentionPolicy{MaxAge: 24 * time.Hour}.Apply(hourly(100), now)
	if len(got) != 25 { // 24 hours back, inclusive
		t.Errorf("kept %d, want 25", len(got))
	}
	for _, r := range got {
		if now.Sub(time.Unix(r.Unix, 0)) > 24*time.Hour {
			t.Errorf("a record older than the limit survived: %v", time.Unix(r.Unix, 0))
		}
	}
}

// TestDownsampleThinsWithoutDeleting is the point of the item: a year of hourly
// checks has minute-by-minute detail nobody will look at, and a shape a trend
// needs. Thinning keeps the shape.
func TestDownsampleThinsWithoutDeleting(t *testing.T) {
	in := hourly(24 * 10) // ten days
	got := RetentionPolicy{DownsampleAfter: 48 * time.Hour}.Apply(in, now)

	if len(got) >= len(in) {
		t.Fatalf("nothing was thinned: %d → %d", len(in), len(got))
	}
	// Everything inside the window is still there at full resolution.
	recent := 0
	for _, r := range got {
		if now.Sub(time.Unix(r.Unix, 0)) <= 48*time.Hour {
			recent++
		}
	}
	if recent != 49 {
		t.Errorf("the recent window holds %d records, want 49 at full resolution", recent)
	}
	// And the older days survive as one record each, not zero.
	older := len(got) - recent
	if older < 7 || older > 9 {
		t.Errorf("older days collapsed to %d records, want about one per day", older)
	}
	// The oldest point is still there: the trend must not lose its start.
	if got[0].Unix > in[0].Unix+int64(24*time.Hour/time.Second) {
		t.Error("the start of the history was thrown away, not thinned")
	}
}

// TestDownsampleKeepsTheLastOfEachDay — a day's final state is what a trend
// should carry, and what a human means by "how was it on Tuesday".
func TestDownsampleKeepsTheLastOfEachDay(t *testing.T) {
	in := hourly(24 * 3)
	window := time.Hour
	cutoff := now.Add(-window)
	got := RetentionPolicy{DownsampleAfter: window}.Apply(in, now)

	// The cutoff itself counts as inside the window (the code keeps r.Unix >=
	// cutoff whole), so the boundary comparisons here have to match that or the
	// test drifts one record away from the behaviour it is describing.
	//
	// Only days that lie *entirely* before the cutoff have a single
	// representative. The boundary day is split: its records after the cutoff
	// are kept whole, so its representative is the last one before it — not the
	// day's last overall. Asserting otherwise was the test being wrong, not the
	// downsampling.
	byDay := map[string]int64{}
	dayStraddles := map[string]bool{}
	for _, r := range in {
		at := time.Unix(r.Unix, 0).UTC()
		day := at.Format("2006-01-02")
		if !at.Before(cutoff) {
			dayStraddles[day] = true
			continue
		}
		if r.Unix > byDay[day] {
			byDay[day] = r.Unix
		}
	}
	for _, r := range got {
		at := time.Unix(r.Unix, 0).UTC()
		if !at.Before(cutoff) {
			continue // at or inside the window, kept whole
		}
		day := at.Format("2006-01-02")
		if r.Unix != byDay[day] {
			t.Errorf("kept %v for %s, want the day's last record before the cutoff (%v)",
				at, day, time.Unix(byDay[day], 0).UTC())
		}
	}
	// And a straddling day still contributes its pre-cutoff representative, so
	// the trend does not have a hole where the window begins.
	for day := range dayStraddles {
		if byDay[day] == 0 {
			continue // the day is entirely inside the window
		}
		var found bool
		for _, r := range got {
			if r.Unix == byDay[day] {
				found = true
			}
		}
		if !found {
			t.Errorf("the boundary day %s lost its pre-window representative", day)
		}
	}
}

// TestDownsampleRunsBeforeTheCounts: thinning after a MaxRuns cut would spend
// the budget on recent detail and leave no history at all.
func TestDownsampleRunsBeforeTheCounts(t *testing.T) {
	in := hourly(24 * 30) // a month
	got := RetentionPolicy{DownsampleAfter: 24 * time.Hour, MaxRuns: 40}.Apply(in, now)
	if len(got) > 40 {
		t.Fatalf("kept %d, over the cap", len(got))
	}
	// With thinning first, the kept set still spans weeks rather than a day.
	span := time.Unix(got[len(got)-1].Unix, 0).Sub(time.Unix(got[0].Unix, 0))
	if span < 7*24*time.Hour {
		t.Errorf("the kept history spans only %v — the cap ate the trend", span)
	}
}

func TestCompactRewritesTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "h.jsonl")
	s := Open(path)
	for _, r := range hourly(100) {
		if err := s.Append(r); err != nil {
			t.Fatal(err)
		}
	}
	dropped, err := s.Compact(RetentionPolicy{MaxRuns: 10}, now)
	if err != nil {
		t.Fatal(err)
	}
	if dropped != 90 {
		t.Errorf("dropped %d, want 90", dropped)
	}
	back, err := s.Recent(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(back) != 10 {
		t.Errorf("file holds %d records after compaction, want 10", len(back))
	}
	// Still readable, still stamped.
	for _, r := range back {
		if r.Schema != SchemaVersion {
			t.Errorf("a rewritten record lost its schema stamp: %+v", r)
		}
	}
}

func TestCompactIsANoOpWhenNothingWouldGo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "h.jsonl")
	s := Open(path)
	for _, r := range hourly(5) {
		_ = s.Append(r)
	}
	before, _ := os.ReadFile(path)
	dropped, err := s.Compact(RetentionPolicy{MaxRuns: 100}, now)
	if err != nil || dropped != 0 {
		t.Fatalf("dropped=%d err=%v", dropped, err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Error("the file was rewritten for nothing")
	}
}

// TestCompactRefusesAHistoryItCannotReadCleanly — rewriting one that contains
// lines from a newer checkfleet would silently discard them, which is a worse
// outcome than a large file.
func TestCompactRefusesAHistoryItCannotReadCleanly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "h.jsonl")
	s := Open(path)
	for _, r := range hourly(3) {
		_ = s.Append(r)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"t":9999999999,"sv":99,"f":[]}` + "\n"); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	before, _ := os.ReadFile(path)
	if _, err := s.Compact(RetentionPolicy{MaxRuns: 1}, now); err == nil {
		t.Error("compaction must refuse a history with unreadable lines")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Error("the file was modified despite the refusal")
	}
}

func TestCompactLeavesNoTempFileBehind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "h.jsonl")
	s := Open(path)
	for _, r := range hourly(20) {
		_ = s.Append(r)
	}
	if _, err := s.Compact(RetentionPolicy{MaxRuns: 5}, now); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("directory holds %d files after compaction, want just the history", len(entries))
	}
}
