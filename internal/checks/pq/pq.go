// Package pq embeds pqprobe: which classes of TLS client can still complete a
// handshake with an endpoint, now that post-quantum key exchange (hybrid
// ML-KEM) is on by default in browsers and CDNs (CF-187).
//
// It is an embed rather than a reimplementation on purpose. The distinction the
// whole check rests on — a peer that answers a post-quantum ClientHello with a
// TLS *alert* has parsed it and declined a group, while one that resets, times
// out or vanishes choked on the hello itself and is broken for every client
// that merely offers ML-KEM — has to live in exactly one place. A second copy
// here would be the copy that goes quietly wrong.
//
// Read-only, like pqprobe: it completes TLS handshakes and closes them. No
// request, no application data, no credentials.
package pq

import (
	"context"
	"fmt"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	pqprobe "github.com/Allan-Nava/pqprobe/pq"
)

type Check struct {
	cfg engine.PQConfig
	// probe is injectable for tests; defaults to a live run.
	probe func(ctx context.Context, targets []string, opt pqprobe.Options) ([]pqprobe.Report, error)
}

func New(cfg engine.PQConfig) *Check {
	return &Check{cfg: cfg, probe: pqprobe.Probe}
}

func (c *Check) Name() string { return "pq" }

func (c *Check) Run(ctx context.Context) []engine.Finding {
	opt := pqprobe.Options{
		Profiles:    c.cfg.Profiles,
		Timeout:     time.Duration(c.cfg.TimeoutSeconds) * time.Second,
		Concurrency: c.cfg.Concurrency,
		Socks5:      c.cfg.Socks5,
	}

	reports, err := c.probe(ctx, c.cfg.Targets, opt)
	if err != nil {
		// Something the caller got wrong — an unknown profile, nothing
		// parseable. One row per configured target, because a silent empty run
		// reads as a fleet that is fine.
		out := make([]engine.Finding, 0, len(c.cfg.Targets))
		for _, t := range c.cfg.Targets {
			out = append(out, engine.Finding{
				Check: c.Name(), Target: t, Status: engine.ERROR,
				Message:     fmt.Sprintf("pqprobe could not run: %v", err),
				Remediation: "check the pq block in checkfleet.yml: targets and profile names",
			})
		}
		return out
	}

	var out []engine.Finding
	for _, r := range reports {
		for _, f := range r.Findings {
			// The verdict always, and any finding that is not OK. A healthy
			// endpoint is one green row: the per-profile detail is evidence for
			// a failure, and on a fleet table it would bury the answer.
			if f.Check != "verdict" && f.Status == string(engine.OK) {
				continue
			}
			out = append(out, engine.Finding{
				Check:  c.Name(),
				Target: subject(r.Target, f),
				Status: engine.Status(f.Status),
				// The class leads: it is the sentence somebody quotes.
				Message: message(r, f),
				Value:   f.Value,
				Unit:    f.Unit,
				// pqprobe's hint is the half that says what to do, which is what
				// checkfleet calls a remediation.
				Remediation: f.Hint,
			})
		}
	}
	return out
}

// subject is the target a row is about. pqprobe already qualifies a
// per-profile finding as "host:port/profile"; the others take the check name as
// a suffix so two rows about one endpoint stay distinguishable in a table that
// is keyed by check and target.
func subject(target string, f pqprobe.Finding) string {
	switch {
	case f.Check == "verdict":
		return target
	case f.Target != "" && f.Target != target:
		return f.Target
	default:
		return target + "/" + f.Check
	}
}

func message(r pqprobe.Report, f pqprobe.Finding) string {
	if f.Check == "verdict" {
		return f.Message
	}
	return fmt.Sprintf("%s: %s", f.Check, f.Message)
}
