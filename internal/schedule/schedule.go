// Package schedule runs each check module on its own cadence (CF-178).
//
// A certificate does not change in thirty seconds and an HTTP endpoint does,
// but `serve` and `watch` had a single --interval for everything. The cost is
// paid twice: needless load on the fleet, and a graph whose resolution says
// more about the poll rate than about the system.
package schedule

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// Entry is one module and how often to run it.
type Entry struct {
	Job engine.Job
	// Every is the module's own cadence. Zero means the scheduler's base
	// interval, so a config that sets nothing behaves exactly as before.
	Every time.Duration
}

// Sample is one module's most recent result.
type Sample struct {
	Findings []engine.Finding
	At       time.Time
	Duration time.Duration
}

// Scheduler keeps the latest sample per module and decides which are due.
//
// It holds results rather than recomputing them, because that is the whole
// point: a module on an hourly cadence must keep contributing its findings to
// /metrics during the fifty-nine minutes it is not running. The alternative —
// dropping it from the output between runs — would look like the check
// disappearing, which is the failure mode this project keeps refusing.
type Scheduler struct {
	base    time.Duration
	entries []Entry
	limit   int

	mu      sync.Mutex
	samples map[string]Sample
}

// New returns a scheduler over the entries. base is the fallback cadence for
// entries that declare none, and limit is the concurrency cap (0 = unbounded).
func New(entries []Entry, base time.Duration, limit int) *Scheduler {
	return &Scheduler{base: base, entries: entries, limit: limit, samples: map[string]Sample{}}
}

// every returns the effective cadence of an entry.
func (s *Scheduler) every(e Entry) time.Duration {
	if e.Every > 0 {
		return e.Every
	}
	return s.base
}

// Due returns the entries whose cadence has elapsed at now — including any that
// have never run, so the first tick covers everything.
func (s *Scheduler) Due(now time.Time) []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	var due []Entry
	for _, e := range s.entries {
		last, seen := s.samples[e.Job.Check.Name()]
		if !seen || now.Sub(last.At) >= s.every(e) {
			due = append(due, e)
		}
	}
	return due
}

// RunDue runs the modules that are due and stores their samples.
func (s *Scheduler) RunDue(ctx context.Context, now time.Time) int {
	due := s.Due(now)
	if len(due) == 0 {
		return 0
	}
	jobs := make([]engine.Job, len(due))
	for i, e := range due {
		jobs[i] = e.Job
	}
	res := engine.RunJobsLimited(ctx, jobs, s.limit)

	// Group by check name: RunJobsLimited flattens, and a module contributes
	// several findings.
	byCheck := map[string][]engine.Finding{}
	for _, f := range res.Findings {
		byCheck[f.Check] = append(byCheck[f.Check], f)
	}
	s.mu.Lock()
	for _, e := range due {
		name := e.Job.Check.Name()
		s.samples[name] = Sample{Findings: byCheck[name], At: now, Duration: res.Duration}
	}
	s.mu.Unlock()
	return len(due)
}

// Result merges the latest sample of every module into one Result.
//
// Started is the *oldest* sample in the merge, not the newest: it is the point
// from which the picture is complete, and reporting the newest would claim a
// freshness the slow modules do not have.
func (s *Scheduler) Result(labels map[string]string) engine.Result {
	s.mu.Lock()
	defer s.mu.Unlock()

	var findings []engine.Finding
	var oldest time.Time
	var total time.Duration
	names := make([]string, 0, len(s.samples))
	for name := range s.samples {
		names = append(names, name)
	}
	sort.Strings(names) // deterministic before the engine's own sort
	for _, name := range names {
		sm := s.samples[name]
		findings = append(findings, sm.Findings...)
		total += sm.Duration
		if oldest.IsZero() || sm.At.Before(oldest) {
			oldest = sm.At
		}
	}
	return engine.Result{
		Findings: engine.SortFindings(engine.Dedup(findings)),
		Started:  oldest,
		Duration: total,
		Labels:   labels,
	}
}

// Ages returns how long ago each module last produced a sample.
//
// With per-module cadences a stale metric is normal, so "stale" stops being a
// useful alarm and the age becomes the thing to alert on. Exposing it is what
// keeps an hourly module from looking silently frozen.
func (s *Scheduler) Ages(now time.Time) map[string]time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]time.Duration, len(s.samples))
	for name, sm := range s.samples {
		out[name] = now.Sub(sm.At)
	}
	return out
}

// Cadences reports the effective interval of every module, for the status page.
func (s *Scheduler) Cadences() map[string]time.Duration {
	out := make(map[string]time.Duration, len(s.entries))
	for _, e := range s.entries {
		out[e.Job.Check.Name()] = s.every(e)
	}
	return out
}

// Tick is the smallest useful wake-up: the shortest cadence in play, so no
// module waits longer than it asked for. Capped below at a second so a
// misconfigured interval cannot spin.
func (s *Scheduler) Tick() time.Duration {
	shortest := s.base
	for _, e := range s.entries {
		if d := s.every(e); d > 0 && d < shortest {
			shortest = d
		}
	}
	if shortest < time.Second {
		return time.Second
	}
	return shortest
}
