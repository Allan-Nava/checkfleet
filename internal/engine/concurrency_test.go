package engine

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// blockingCheck records how many checks run at once (peak) and holds each one
// until released, so a test can observe the concurrency ceiling.
type blockingCheck struct {
	name    string
	cur     *int32
	peak    *int32
	release <-chan struct{}
}

func (b blockingCheck) Name() string { return b.name }
func (b blockingCheck) Run(ctx context.Context) []Finding {
	n := atomic.AddInt32(b.cur, 1)
	for {
		p := atomic.LoadInt32(b.peak)
		if n <= p || atomic.CompareAndSwapInt32(b.peak, p, n) {
			break
		}
	}
	<-b.release // hold the slot until the test lets go
	atomic.AddInt32(b.cur, -1)
	return []Finding{{Check: b.name, Target: "t", Status: OK, Message: "ok"}}
}

func peakOf(t *testing.T, n, limit int) int32 {
	t.Helper()
	var cur, peak int32
	release := make(chan struct{})
	checks := make([]Check, n)
	for i := range checks {
		checks[i] = blockingCheck{name: "c" + string(rune('a'+i)), cur: &cur, peak: &peak, release: release}
	}
	done := make(chan Result, 1)
	go func() { done <- RunWithLimit(context.Background(), checks, Options{Timeout: time.Second}, limit) }()

	// Wait until concurrency stops climbing (all runnable slots are filled),
	// then release everyone.
	deadline := time.Now().Add(2 * time.Second)
	for {
		want := int32(n)
		if limit > 0 && int32(limit) < want {
			want = int32(limit)
		}
		if atomic.LoadInt32(&cur) >= want || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	close(release)
	res := <-done
	if len(res.Findings) != n {
		t.Fatalf("got %d findings, want %d", len(res.Findings), n)
	}
	return atomic.LoadInt32(&peak)
}

// With a cap of 2, no more than 2 of 6 checks ever run at once.
func TestRunJobsLimitedCapsConcurrency(t *testing.T) {
	if peak := peakOf(t, 6, 2); peak > 2 {
		t.Fatalf("peak concurrency = %d, want <= 2", peak)
	}
}

// With no cap, all checks run at once (the historical behaviour).
func TestRunJobsLimitedUnbounded(t *testing.T) {
	if peak := peakOf(t, 6, 0); peak != 6 {
		t.Fatalf("peak concurrency = %d, want 6 (unbounded)", peak)
	}
}

// A cap larger than the job count doesn't stall anything.
func TestRunJobsLimitedCapAboveCount(t *testing.T) {
	if peak := peakOf(t, 3, 10); peak != 3 {
		t.Fatalf("peak concurrency = %d, want 3", peak)
	}
}

// The cap doesn't change results or ordering: a limited run returns the same
// findings as an unlimited one.
func TestRunJobsLimitedSameResult(t *testing.T) {
	mk := func() []Check {
		return []Check{
			stubCheck{"b", []Finding{{Check: "b", Target: "t", Status: WARN, Message: "w"}}},
			stubCheck{"a", []Finding{{Check: "a", Target: "t", Status: ERROR, Message: "e"}}},
		}
	}
	unlimited := RunWithLimit(context.Background(), mk(), Options{Timeout: time.Second}, 0)
	limited := RunWithLimit(context.Background(), mk(), Options{Timeout: time.Second}, 1)
	if len(limited.Findings) != len(unlimited.Findings) {
		t.Fatalf("len differ: %d vs %d", len(limited.Findings), len(unlimited.Findings))
	}
	for i := range unlimited.Findings {
		if limited.Findings[i].Check != unlimited.Findings[i].Check {
			t.Fatalf("order differs at %d: %q vs %q", i, limited.Findings[i].Check, unlimited.Findings[i].Check)
		}
	}
}
