// Package engine defines the check contract and the runner.
// A Check produces Findings; the runner executes every registered check with
// a shared timeout and aggregates results. Output rendering lives in
// internal/output, so checks stay pure and testable.
package engine

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Status of a single finding. Severity order: OK < WARN < BAD < ERROR.
type Status string

const (
	OK    Status = "OK"
	WARN  Status = "WARN"
	BAD   Status = "BAD"
	ERROR Status = "ERROR" // the check itself could not run against the target
)

var severity = map[Status]int{OK: 0, WARN: 1, BAD: 2, ERROR: 3}

// Finding is one observation about one target.
type Finding struct {
	Check   string `json:"check"`
	Target  string `json:"target"`
	Status  Status `json:"status"`
	Message string `json:"message"`
}

// Check is implemented by every module (certs, http, ...).
type Check interface {
	Name() string
	Run(ctx context.Context) []Finding
}

// Result aggregates the findings of a run.
type Result struct {
	Findings []Finding     `json:"findings"`
	Started  time.Time     `json:"started"`
	Duration time.Duration `json:"duration_ns"`
}

// Run executes the checks sequentially, each bounded by timeout.
// Findings are sorted by severity (worst first), then check, then target.
// Options tunes a run.
type Options struct {
	Timeout time.Duration // per-check (and per-attempt) deadline
	Retries int           // extra attempts for a check that produced ERROR findings
	Backoff time.Duration // base backoff between attempts (doubles each retry)
}

// Run executes the checks with only a timeout (no retries).
func Run(ctx context.Context, checks []Check, timeout time.Duration) Result {
	return RunWith(ctx, checks, Options{Timeout: timeout})
}

// RunWith executes the checks concurrently under opts. Results are collected
// per-check by index and flattened in check order, so the output is
// deterministic regardless of completion order (the stable sort below then
// orders by severity). Checks whose result contains an ERROR finding are
// retried up to opts.Retries times with exponential backoff.
func RunWith(ctx context.Context, checks []Check, opts Options) Result {
	started := time.Now()
	perCheck := make([][]Finding, len(checks))
	var wg sync.WaitGroup
	for i, c := range checks {
		wg.Add(1)
		go func(i int, c Check) {
			defer wg.Done()
			perCheck[i] = runWithRetry(ctx, c, opts)
		}(i, c)
	}
	wg.Wait()
	var findings []Finding
	for _, fs := range perCheck {
		findings = append(findings, fs...)
	}
	sort.SliceStable(findings, func(i, j int) bool {
		if severity[findings[i].Status] != severity[findings[j].Status] {
			return severity[findings[i].Status] > severity[findings[j].Status]
		}
		if findings[i].Check != findings[j].Check {
			return findings[i].Check < findings[j].Check
		}
		return findings[i].Target < findings[j].Target
	})
	return Result{Findings: findings, Started: started, Duration: time.Since(started)}
}

// runWithRetry runs one check, retrying (with exponential backoff) while its
// result still contains an ERROR finding — a check that couldn't measure
// (network, handshake) is often a transient.
func runWithRetry(ctx context.Context, c Check, opts Options) []Finding {
	attempts := 1 + opts.Retries
	var res []Finding
	for a := 0; a < attempts; a++ {
		cctx, cancel := context.WithTimeout(ctx, opts.Timeout)
		res = c.Run(cctx)
		cancel()
		if a == attempts-1 || !hasError(res) {
			break
		}
		select {
		case <-time.After(opts.Backoff << a):
		case <-ctx.Done():
			return res
		}
	}
	return res
}

func hasError(findings []Finding) bool {
	for _, f := range findings {
		if f.Status == ERROR {
			return true
		}
	}
	return false
}

// Summarize counts findings per status.
func Summarize(findings []Finding) map[Status]int {
	m := map[Status]int{}
	for _, f := range findings {
		m[f.Status]++
	}
	return m
}

// Worst returns the most severe status present (OK for an empty list).
func Worst(findings []Finding) Status {
	worst := OK
	for _, f := range findings {
		if severity[f.Status] > severity[worst] {
			worst = f.Status
		}
	}
	return worst
}
