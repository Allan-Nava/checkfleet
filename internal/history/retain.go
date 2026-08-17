package history

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// RetentionPolicy bounds a history file that would otherwise grow forever
// (CF-177).
//
// Append-only is the right default — losing a run to a rotation you forgot is
// worse than a large file — but "and in a year?" has never had an answer, and
// the M30 analyses re-read the whole file.
//
// The interesting part is DownsampleAfter: old runs are *thinned*, not deleted.
// A year of hourly checks is 8,760 records whose minute-by-minute detail nobody
// will ever look at, while the shape of the year is exactly what a trend needs.
// Keeping one run per day past the recent window preserves the shape and drops
// the bulk.
type RetentionPolicy struct {
	// MaxRuns keeps at most this many records (0 = unlimited).
	MaxRuns int
	// MaxAge drops records older than this (0 = unlimited).
	MaxAge time.Duration
	// DownsampleAfter keeps every run inside this window and one per day
	// outside it (0 = keep everything at full resolution).
	DownsampleAfter time.Duration
}

// Set reports whether the policy would do anything.
func (p RetentionPolicy) Set() bool {
	return p.MaxRuns > 0 || p.MaxAge > 0 || p.DownsampleAfter > 0
}

// Apply returns the records the policy keeps, oldest first. Pure: the caller
// decides whether to write them.
//
// Order matters. Downsampling runs first so the age and count limits are
// applied to what will actually be kept — thinning after a MaxRuns cut would
// spend the budget on recent detail and leave no history at all.
func (p RetentionPolicy) Apply(records []Record, now time.Time) []Record {
	if !p.Set() || len(records) == 0 {
		return records
	}
	out := append([]Record(nil), records...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Unix < out[j].Unix })

	if p.DownsampleAfter > 0 {
		out = downsample(out, now.Add(-p.DownsampleAfter))
	}
	if p.MaxAge > 0 {
		cutoff := now.Add(-p.MaxAge).Unix()
		kept := out[:0:0]
		for _, r := range out {
			if r.Unix >= cutoff {
				kept = append(kept, r)
			}
		}
		out = kept
	}
	if p.MaxRuns > 0 && len(out) > p.MaxRuns {
		out = out[len(out)-p.MaxRuns:]
	}
	return out
}

// downsample keeps every record at or after boundary, and the *last* record of
// each day before it.
//
// The last, not the first: a day's final state is the one a trend should carry,
// and it is also what a human means by "how was it on Tuesday".
func downsample(records []Record, boundary time.Time) []Record {
	cutoff := boundary.Unix()
	out := records[:0:0]
	var lastDay string
	var pending *Record

	flush := func() {
		if pending != nil {
			out = append(out, *pending)
			pending = nil
		}
	}
	for i := range records {
		r := records[i]
		if r.Unix >= cutoff {
			flush()
			out = append(out, r)
			continue
		}
		day := time.Unix(r.Unix, 0).UTC().Format("2006-01-02")
		if day != lastDay {
			flush()
			lastDay = day
		}
		cp := r
		pending = &cp
	}
	flush()
	return out
}

// Compact rewrites the history file with only the records the policy keeps.
//
// Crash-safe in the same way Append is not allowed to break: the survivors go
// to a temporary file in the same directory, which is then renamed over the
// original. A crash mid-write leaves the old file intact — losing a monitoring
// history to a truncation is a bad way to save disk space.
//
// Returns how many records were dropped.
func (s *Store) Compact(p RetentionPolicy, now time.Time) (int, error) {
	if !p.Set() {
		return 0, nil
	}
	records, err := s.Recent(0)
	if err != nil {
		// Recent returns what it could read alongside the error (a newer schema
		// on some lines). Rewriting then would silently discard those lines, so
		// this refuses instead.
		return 0, fmt.Errorf("compact %s: refusing to rewrite a history that did not read cleanly: %w", s.path, err)
	}
	if len(records) == 0 {
		return 0, nil
	}
	kept := p.Apply(records, now)
	if len(kept) == len(records) {
		return 0, nil
	}

	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".checkfleet-history-*")
	if err != nil {
		return 0, err
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()

	w := bufio.NewWriter(tmp)
	for _, r := range kept {
		if r.Schema == 0 {
			r.Schema = SchemaVersion
		}
		line, err := json.Marshal(r)
		if err != nil {
			_ = tmp.Close()
			return 0, err
		}
		if _, err := w.Write(append(line, '\n')); err != nil {
			_ = tmp.Close()
			return 0, err
		}
	}
	if err := w.Flush(); err != nil {
		_ = tmp.Close()
		return 0, err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return 0, err
	}
	if err := tmp.Close(); err != nil {
		return 0, err
	}
	if err := os.Chmod(name, 0o644); err != nil {
		return 0, err
	}
	if err := os.Rename(name, s.path); err != nil {
		return 0, err
	}
	return len(records) - len(kept), nil
}
