package schedule

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// counted is a check that records how many times it ran.
type counted struct {
	name string
	runs atomic.Int64
	st   engine.Status
}

func (c *counted) Name() string { return c.name }
func (c *counted) Run(context.Context) []engine.Finding {
	c.runs.Add(1)
	st := c.st
	if st == "" {
		st = engine.OK
	}
	return []engine.Finding{{Check: c.name, Target: "t", Status: st, Message: "m"}}
}

func entry(name string, every time.Duration) (Entry, *counted) {
	c := &counted{name: name}
	return Entry{Job: engine.Job{Check: c, Opts: engine.Options{Timeout: time.Second}}, Every: every}, c
}

var t0 = time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC)

func TestEverythingIsDueOnTheFirstTick(t *testing.T) {
	e1, c1 := entry("certs", time.Hour)
	e2, c2 := entry("http", 30*time.Second)
	s := New([]Entry{e1, e2}, time.Minute, 0)

	if n := s.RunDue(context.Background(), t0); n != 2 {
		t.Fatalf("ran %d modules on the first tick, want 2", n)
	}
	if c1.runs.Load() != 1 || c2.runs.Load() != 1 {
		t.Errorf("runs: certs=%d http=%d", c1.runs.Load(), c2.runs.Load())
	}
}

// TestASlowModuleIsNotRunAgainEarly is the point of the feature: a certificate
// does not change in thirty seconds, and polling it as if it did is load on the
// fleet for nothing.
func TestASlowModuleIsNotRunAgainEarly(t *testing.T) {
	slow, cs := entry("certs", time.Hour)
	fast, cf := entry("http", 30*time.Second)
	s := New([]Entry{slow, fast}, time.Minute, 0)
	s.RunDue(context.Background(), t0)

	for i := 1; i <= 10; i++ { // ten ticks of 30s = five minutes
		s.RunDue(context.Background(), t0.Add(time.Duration(i)*30*time.Second))
	}
	if got := cs.runs.Load(); got != 1 {
		t.Errorf("the hourly module ran %d times in five minutes, want 1", got)
	}
	if got := cf.runs.Load(); got != 11 {
		t.Errorf("the 30s module ran %d times, want 11", got)
	}
}

func TestTheSlowModuleRunsAgainWhenItsCadenceElapses(t *testing.T) {
	slow, cs := entry("certs", time.Hour)
	s := New([]Entry{slow}, time.Minute, 0)
	s.RunDue(context.Background(), t0)
	s.RunDue(context.Background(), t0.Add(59*time.Minute))
	if cs.runs.Load() != 1 {
		t.Errorf("ran early: %d", cs.runs.Load())
	}
	s.RunDue(context.Background(), t0.Add(time.Hour))
	if cs.runs.Load() != 2 {
		t.Errorf("did not run at its cadence: %d", cs.runs.Load())
	}
}

// TestAModuleKeepsContributingBetweenRuns — dropping it from the output while
// it waits would look exactly like the check disappearing.
func TestAModuleKeepsContributingBetweenRuns(t *testing.T) {
	slow, _ := entry("certs", time.Hour)
	fast, _ := entry("http", 30*time.Second)
	s := New([]Entry{slow, fast}, time.Minute, 0)
	s.RunDue(context.Background(), t0)
	s.RunDue(context.Background(), t0.Add(30*time.Second))

	res := s.Result(nil)
	var sawCerts bool
	for _, f := range res.Findings {
		if f.Check == "certs" {
			sawCerts = true
		}
	}
	if !sawCerts {
		t.Errorf("the hourly module vanished from the merged result: %+v", res.Findings)
	}
	if len(res.Findings) != 2 {
		t.Errorf("merged %d findings, want one per module", len(res.Findings))
	}
}

// TestStartedIsTheOldestSample: reporting the newest would claim a freshness
// the slow modules do not have.
func TestStartedIsTheOldestSample(t *testing.T) {
	slow, _ := entry("certs", time.Hour)
	fast, _ := entry("http", 30*time.Second)
	s := New([]Entry{slow, fast}, time.Minute, 0)
	s.RunDue(context.Background(), t0)
	later := t0.Add(30 * time.Second)
	s.RunDue(context.Background(), later)

	if got := s.Result(nil).Started; !got.Equal(t0) {
		t.Errorf("Started = %v, want the oldest sample %v", got, t0)
	}
}

func TestAgesReportPerModuleStaleness(t *testing.T) {
	slow, _ := entry("certs", time.Hour)
	fast, _ := entry("http", 30*time.Second)
	s := New([]Entry{slow, fast}, time.Minute, 0)
	s.RunDue(context.Background(), t0)
	at := t0.Add(45 * time.Minute)
	s.RunDue(context.Background(), at)

	ages := s.Ages(at)
	if ages["certs"] != 45*time.Minute {
		t.Errorf("certs age = %v, want 45m", ages["certs"])
	}
	if ages["http"] != 0 {
		t.Errorf("http age = %v, want 0 (it just ran)", ages["http"])
	}
}

// TestZeroCadenceFallsBackToTheBase — a config that sets nothing must behave
// exactly as it did before this feature.
func TestZeroCadenceFallsBackToTheBase(t *testing.T) {
	e, c := entry("http", 0)
	s := New([]Entry{e}, time.Minute, 0)
	s.RunDue(context.Background(), t0)
	s.RunDue(context.Background(), t0.Add(30*time.Second))
	if c.runs.Load() != 1 {
		t.Errorf("ran at %d, want 1 — it should follow the base interval", c.runs.Load())
	}
	s.RunDue(context.Background(), t0.Add(time.Minute))
	if c.runs.Load() != 2 {
		t.Errorf("did not follow the base interval: %d", c.runs.Load())
	}
	if got := s.Cadences()["http"]; got != time.Minute {
		t.Errorf("cadence = %v, want the base", got)
	}
}

// TestTickIsTheShortestCadence: waking on the base interval would make a 10s
// module fire every 60s, which is the setting quietly not working.
func TestTickIsTheShortestCadence(t *testing.T) {
	a, _ := entry("http", 10*time.Second)
	b, _ := entry("certs", time.Hour)
	s := New([]Entry{a, b}, time.Minute, 0)
	if got := s.Tick(); got != 10*time.Second {
		t.Errorf("tick = %v, want the shortest cadence", got)
	}
	// A misconfigured interval must not spin the loop.
	c, _ := entry("x", time.Millisecond)
	if got := New([]Entry{c}, time.Minute, 0).Tick(); got < time.Second {
		t.Errorf("tick = %v, want at least a second", got)
	}
}

func TestEmptySchedulerIsSafe(t *testing.T) {
	s := New(nil, time.Minute, 0)
	if n := s.RunDue(context.Background(), t0); n != 0 {
		t.Errorf("ran %d modules with none configured", n)
	}
	if res := s.Result(nil); len(res.Findings) != 0 {
		t.Errorf("findings from nothing: %+v", res)
	}
}
