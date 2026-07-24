package engine

import (
	"context"
	"testing"
	"time"
)

func TestWorstAndSummarize(t *testing.T) {
	findings := []Finding{
		{Status: OK}, {Status: WARN}, {Status: OK}, {Status: BAD},
	}
	if w := Worst(findings); w != BAD {
		t.Errorf("Worst: atteso BAD, avuto %s", w)
	}
	if w := Worst(nil); w != OK {
		t.Errorf("Worst(nil): atteso OK, avuto %s", w)
	}
	s := Summarize(findings)
	if s[OK] != 2 || s[WARN] != 1 || s[BAD] != 1 || s[ERROR] != 0 {
		t.Errorf("Summarize sbagliato: %v", s)
	}
}

// stubCheck returns a fixed set of findings.
type stubCheck struct {
	name     string
	findings []Finding
}

func (s stubCheck) Name() string                  { return s.name }
func (s stubCheck) Run(context.Context) []Finding { return s.findings }

func TestRunSortsWorstFirstStable(t *testing.T) {
	checks := []Check{
		stubCheck{name: "a", findings: []Finding{
			{Check: "a", Target: "t2", Status: OK},
			{Check: "a", Target: "t1", Status: OK},
		}},
		stubCheck{name: "b", findings: []Finding{
			{Check: "b", Target: "x", Status: ERROR},
			{Check: "b", Target: "y", Status: WARN},
			{Check: "b", Target: "z", Status: BAD},
		}},
	}
	res := Run(context.Background(), checks, time.Second)

	wantStatus := []Status{ERROR, BAD, WARN, OK, OK}
	if len(res.Findings) != len(wantStatus) {
		t.Fatalf("attesi %d finding, avuti %d", len(wantStatus), len(res.Findings))
	}
	for i, want := range wantStatus {
		if res.Findings[i].Status != want {
			t.Errorf("posizione %d: atteso %s, avuto %s (%+v)", i, want, res.Findings[i].Status, res.Findings[i])
		}
	}
	// Stable secondary sort: within equal status (OK), by check then target.
	last := res.Findings[len(res.Findings)-2:]
	if last[0].Target != "t1" || last[1].Target != "t2" {
		t.Errorf("ordinamento secondario per target non stabile: %+v", last)
	}
}

func TestRunRespectsTimeout(t *testing.T) {
	slow := funcCheck(func(ctx context.Context) []Finding {
		select {
		case <-time.After(2 * time.Second):
			return []Finding{{Status: OK}}
		case <-ctx.Done():
			return []Finding{{Status: ERROR, Message: ctx.Err().Error()}}
		}
	})
	start := time.Now()
	res := Run(context.Background(), []Check{slow}, 50*time.Millisecond)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("Run non ha rispettato il timeout: %s", elapsed)
	}
	if Worst(res.Findings) != ERROR {
		t.Errorf("atteso ERROR dal timeout, avuto %v", res.Findings)
	}
}

type funcCheck func(context.Context) []Finding

func (f funcCheck) Name() string                      { return "func" }
func (f funcCheck) Run(ctx context.Context) []Finding { return f(ctx) }

func TestRunRunsChecksConcurrently(t *testing.T) {
	// Three checks that each sleep 120ms: run in parallel the total is ~120ms,
	// serial it would be ~360ms.
	sleeper := func(status Status) funcCheck {
		return func(ctx context.Context) []Finding {
			select {
			case <-time.After(120 * time.Millisecond):
			case <-ctx.Done():
			}
			return []Finding{{Status: status}}
		}
	}
	checks := []Check{sleeper(OK), sleeper(WARN), sleeper(BAD)}
	start := time.Now()
	res := Run(context.Background(), checks, time.Second)
	if elapsed := time.Since(start); elapsed > 300*time.Millisecond {
		t.Errorf("i check non girano in parallelo: %s (attesi ~120ms)", elapsed)
	}
	if len(res.Findings) != 3 || Worst(res.Findings) != BAD {
		t.Errorf("risultati inattesi dai check concorrenti: %+v", res.Findings)
	}
}

func TestRunPreservesCheckOrderBeforeSort(t *testing.T) {
	// All findings same severity → order is by the check flattening (stable),
	// which must follow the input check order deterministically.
	mk := func(check string) funcCheck {
		return func(context.Context) []Finding { return []Finding{{Check: check, Status: OK}} }
	}
	res := Run(context.Background(), []Check{mk("a"), mk("b"), mk("c")}, time.Second)
	// Equal severity → secondary sort by check name: a, b, c.
	if res.Findings[0].Check != "a" || res.Findings[1].Check != "b" || res.Findings[2].Check != "c" {
		t.Errorf("ordine non deterministico: %+v", res.Findings)
	}
}
