// Package flow implements the multi-step flow check: an ordered sequence of
// HTTP requests where a value captured from one step is used by the next
// (CF-180).
//
// It exists because half of what an operator actually cares about is not a
// single request. "Login works" is get a token, use it, check the answer. A
// probe that only asks the first of those reports a healthy service while
// nobody can log in.
//
// Two design rules, both non-negotiable:
//
//   - The finding names the step that failed. "The flow is broken" without
//     saying where is a finding that saves nobody any time.
//   - Captured values are never printed. A login flow captures a token, and a
//     finding travels to a terminal, a CI log and a JSON file.
package flow

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

type Check struct {
	cfg engine.FlowConfig
	// tlsErr is a client certificate that could not be loaded. Kept rather than
	// panicking so it can be reported as ERROR — "the check could not measure".
	tlsErr error
	tlsCfg *tls.Config
}

func New(cfg engine.FlowConfig) *Check {
	c := &Check{cfg: cfg}
	if cfg.ClientTLS.Set() {
		c.tlsCfg, c.tlsErr = cfg.ClientTLS.Apply(&tls.Config{MinVersion: tls.VersionTLS12})
	}
	return c
}

func (c *Check) Name() string { return "flow" }

// Run executes every flow. Flows run sequentially rather than in parallel: a
// flow is a login or a checkout, and firing several of those at a production
// system at once is load, not monitoring.
func (c *Check) Run(ctx context.Context) []engine.Finding {
	findings := make([]engine.Finding, 0, len(c.cfg.Flows))
	for _, f := range c.cfg.Flows {
		findings = append(findings, c.run(ctx, f))
	}
	return findings
}

// run executes one flow and collapses it into a single finding. One finding per
// flow, not per step: the question is "does login work", and the answer has to
// carry where it stopped, not fan out into a row per hop.
func (c *Check) run(ctx context.Context, f engine.Flow) engine.Finding {
	target := f.Name
	if target == "" {
		target = "flow"
	}
	fail := func(status engine.Status, format string, args ...any) engine.Finding {
		return engine.Finding{Check: c.Name(), Target: target, Status: status,
			Message: fmt.Sprintf(format, args...)}
	}

	if c.tlsErr != nil {
		return fail(engine.ERROR, "client certificate: %v", c.tlsErr)
	}
	if len(f.Steps) == 0 {
		return fail(engine.ERROR, "no steps configured")
	}

	client, err := c.clientFor(f)
	if err != nil {
		return fail(engine.ERROR, "%v", err)
	}

	captured := map[string]string{}
	var slowSteps []string
	start := time.Now()

	for i, s := range f.Steps {
		// 1-based and with the total, so the message reads the way an operator
		// counts steps.
		where := fmt.Sprintf("step %d/%d %q", i+1, len(f.Steps), stepName(s, i))

		took, err := c.step(ctx, client, s, captured)
		if err != nil {
			// A failed step stops the flow: every later step would fail for the
			// same reason and bury the one that matters.
			return fail(statusFor(err), "%s: %v", where, err)
		}
		if s.MaxLatencyMS > 0 && took > time.Duration(s.MaxLatencyMS)*time.Millisecond {
			slowSteps = append(slowSteps, fmt.Sprintf("%s took %dms (max %dms)",
				where, took.Milliseconds(), s.MaxLatencyMS))
		}
	}

	total := time.Since(start)
	summary := fmt.Sprintf("%d step(s) ok in %dms", len(f.Steps), total.Milliseconds())

	if f.MaxLatencyMS > 0 && total > time.Duration(f.MaxLatencyMS)*time.Millisecond {
		return fail(engine.WARN, "%s, over the %dms budget", summary, f.MaxLatencyMS)
	}
	if len(slowSteps) > 0 {
		return fail(engine.WARN, "%s, but %s", summary, strings.Join(slowSteps, "; "))
	}
	return fail(engine.OK, "%s", summary)
}

func stepName(s engine.FlowStep, i int) string {
	if s.Name != "" {
		return s.Name
	}
	return fmt.Sprintf("step %d", i+1)
}

// clientFor builds the client for one flow, with a cookie jar when the flow
// keeps a session.
func (c *Check) clientFor(f engine.Flow) (*http.Client, error) {
	client := &http.Client{}
	if f.Insecure || c.tlsCfg != nil {
		tc := c.tlsCfg
		if tc == nil {
			tc = &tls.Config{MinVersion: tls.VersionTLS12}
		} else {
			tc = tc.Clone()
		}
		if f.Insecure {
			tc.InsecureSkipVerify = true
		}
		client.Transport = &http.Transport{TLSClientConfig: tc}
	}
	if f.KeepCookies {
		jar, err := cookiejar.New(nil)
		if err != nil {
			return nil, fmt.Errorf("cookie jar: %w", err)
		}
		client.Jar = jar
	}
	return client, nil
}
