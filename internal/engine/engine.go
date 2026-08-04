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

// AtLeast reports whether s is at or above threshold in the severity order
// OK < WARN < BAD < ERROR. An empty threshold is satisfied by anything, since
// severity[""] is the zero value — callers that mean "no threshold at all"
// must test for "" themselves rather than relying on this.
func AtLeast(s, threshold Status) bool {
	return severity[s] >= severity[threshold]
}

// Finding is one observation about one target.
//
// Value/Unit are optional: a module that measures a scalar (latency in ms,
// days-to-expiry, replication lag in seconds) may attach it so the GUI can plot
// the metric over time (CF-91). They stay nil/"" for modules that don't, and
// the output renderers ignore them, so this is backward-compatible.
// Runbook/Remediation are optional operator hints — the procedure URL and a
// short "what to do" note — attached after the run by ApplyRunbooks from the
// config (CF-124). Modules never set them: they are operational text from the
// operator's own config, never credentials. Renderers that don't know about
// them ignore them, so this stays backward-compatible.
type Finding struct {
	Check       string   `json:"check"`
	Target      string   `json:"target"`
	Status      Status   `json:"status"`
	Message     string   `json:"message"`
	Value       *float64 `json:"value,omitempty"`
	Unit        string   `json:"unit,omitempty"`
	Runbook     string   `json:"runbook,omitempty"`
	Remediation string   `json:"remediation,omitempty"`
}

// Num returns a pointer to v, for setting Finding.Value inline.
func Num(v float64) *float64 { return &v }

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
	// Labels are global key/value metadata (env, region, …) carried into the
	// outputs for routing/dashboards (CF-119). Set by the caller after the run.
	Labels map[string]string `json:"labels,omitempty"`
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

// Job pairs a check with the options it should run under, so different modules
// can have different timeouts/retries in the same run (CF-84).
type Job struct {
	Check Check
	Opts  Options
}

// RunWith executes the checks concurrently under a single Options. It is the
// uniform-options entry point (used by the desktop app); RunJobs is the
// per-check-options variant.
func RunWith(ctx context.Context, checks []Check, opts Options) Result {
	return RunWithLimit(ctx, checks, opts, 0)
}

// RunWithLimit is RunWith with a global concurrency cap (CF-116): at most
// maxConcurrency checks run at once (0 = unbounded).
func RunWithLimit(ctx context.Context, checks []Check, opts Options, maxConcurrency int) Result {
	jobs := make([]Job, len(checks))
	for i, c := range checks {
		jobs[i] = Job{Check: c, Opts: opts}
	}
	return RunJobsLimited(ctx, jobs, maxConcurrency)
}

// RunJobs executes each job concurrently under its own Options, with no global
// concurrency cap.
func RunJobs(ctx context.Context, jobs []Job) Result {
	return RunJobsLimited(ctx, jobs, 0)
}

// RunJobsLimited executes each job concurrently under its own Options, capping
// the number running at once to maxConcurrency (CF-116). With maxConcurrency <= 0
// there is no cap (every job starts immediately, the historical behaviour); the
// cap sits ABOVE any per-module concurrency a check applies internally, so a
// fleet of hundreds of targets won't open hundreds of connections at once.
// Results are collected per-job by index and flattened in job order, so the
// output is deterministic regardless of completion order (the stable sort below
// then orders by severity). A job whose result contains an ERROR finding is
// retried up to its Opts.Retries times with exponential backoff.
func RunJobsLimited(ctx context.Context, jobs []Job, maxConcurrency int) Result {
	started := time.Now()
	perCheck := make([][]Finding, len(jobs))
	var sem chan struct{}
	if maxConcurrency > 0 {
		sem = make(chan struct{}, maxConcurrency)
	}
	var wg sync.WaitGroup
	for i, j := range jobs {
		wg.Add(1)
		go func(i int, j Job) {
			defer wg.Done()
			if sem != nil {
				sem <- struct{}{}        // acquire a slot (blocks when the cap is reached)
				defer func() { <-sem }() // release it
			}
			perCheck[i] = runWithRetry(ctx, j.Check, j.Opts)
		}(i, j)
	}
	wg.Wait()
	var findings []Finding
	for _, fs := range perCheck {
		findings = append(findings, fs...)
	}
	findings = SortFindings(Dedup(findings))
	return Result{Findings: findings, Started: started, Duration: time.Since(started)}
}

// SortFindings orders findings worst-first, then by check, then by target, with
// a stable sort so equal keys keep their input order.
//
// This ordering is a de-facto API: anything parsing the text output relies on
// the first line being the thing to look at. It is exported so other producers
// of findings (the doctor command) present them the same way instead of
// reimplementing the comparison.
func SortFindings(findings []Finding) []Finding {
	sort.SliceStable(findings, func(i, j int) bool {
		if severity[findings[i].Status] != severity[findings[j].Status] {
			return severity[findings[i].Status] > severity[findings[j].Status]
		}
		if findings[i].Check != findings[j].Check {
			return findings[i].Check < findings[j].Check
		}
		return findings[i].Target < findings[j].Target
	})
	return findings
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

// Dedup removes exact-duplicate findings (same check, target, status and
// message), keeping the first occurrence and preserving order. Duplicates arise
// when the same target is listed twice or a module emits a finding twice; the
// output should carry each distinct observation once.
func Dedup(findings []Finding) []Finding {
	seen := make(map[Finding]bool, len(findings))
	out := findings[:0:0]
	for _, f := range findings {
		if seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	return out
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
