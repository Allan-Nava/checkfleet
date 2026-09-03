package pq

import (
	"context"
	"errors"
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	pqprobe "github.com/Allan-Nava/pqprobe/pq"
)

// CF-187. The module embeds pqprobe rather than reimplementing it: the
// alert-versus-reset classification is the one thing in that tool that has to
// live in exactly one place, and a second copy here would be the copy that goes
// wrong quietly.
func check(reports []pqprobe.Report, err error) *Check {
	c := New(engine.PQConfig{Targets: []string{"origin.example"}})
	c.probe = func(context.Context, []string, pqprobe.Options) ([]pqprobe.Report, error) {
		return reports, err
	}
	return c
}

func find(t *testing.T, fs []engine.Finding, target string) engine.Finding {
	t.Helper()
	for _, f := range fs {
		if f.Target == target {
			return f
		}
	}
	t.Fatalf("no finding for %q in %+v", target, fs)
	return engine.Finding{}
}

// A healthy endpoint is one green row, not five: the per-profile detail is
// evidence for a failure, and on a fleet table it would bury the answer.
func TestAReadyEndpointIsOneOKRow(t *testing.T) {
	c := check([]pqprobe.Report{{
		Target: "origin.example:443", Class: "pq-ready", Worst: "OK",
		Findings: []pqprobe.Finding{
			{Check: "verdict", Target: "origin.example:443", Status: "OK",
				Message: "pq-ready — post-quantum key exchange works", Hint: "nothing to do"},
			{Check: "handshake", Target: "origin.example:443/classic", Status: "OK", Message: "TLS 1.3"},
			{Check: "handshake", Target: "origin.example:443/pq-only", Status: "OK", Message: "TLS 1.3"},
		},
	}}, nil)

	fs := c.Run(context.Background())
	if len(fs) != 1 {
		t.Fatalf("got %d findings, want one row for a healthy endpoint: %+v", len(fs), fs)
	}
	f := fs[0]
	if f.Check != "pq" {
		t.Errorf("check = %q, want pq", f.Check)
	}
	if f.Status != engine.OK {
		t.Errorf("status = %s, want OK", f.Status)
	}
	if f.Target != "origin.example:443" {
		t.Errorf("target = %q", f.Target)
	}
	if f.Message == "" {
		t.Error("no message")
	}
}

// A failure keeps its evidence: the verdict says what class it is, and the
// handshake that produced it stays visible — that pair is the whole argument
// somebody takes to a CDN vendor.
func TestAnIntolerantEndpointKeepsItsEvidence(t *testing.T) {
	c := check([]pqprobe.Report{{
		Target: "origin.example:443", Class: "pq-intolerant", Worst: "BAD",
		Findings: []pqprobe.Finding{
			{Check: "verdict", Target: "origin.example:443", Status: "BAD",
				Message: "pq-intolerant — capable clients cannot connect",
				Hint:    "look at what the ClientHello has to cross"},
			{Check: "handshake", Target: "origin.example:443/pq-preferred", Status: "WARN",
				Message: "no handshake (reset)", Hint: "an abrupt end means no alert"},
			{Check: "handshake", Target: "origin.example:443/classic", Status: "OK", Message: "TLS 1.3"},
		},
	}}, nil)

	fs := c.Run(context.Background())
	if len(fs) != 2 {
		t.Fatalf("got %d findings, want the verdict and the failing handshake: %+v", len(fs), fs)
	}

	v := find(t, fs, "origin.example:443")
	if v.Status != engine.BAD {
		t.Errorf("verdict status = %s, want BAD", v.Status)
	}
	// pqprobe's hint is the half that says what to do, and checkfleet calls
	// that Remediation — dropping it would leave a row nobody can act on.
	if v.Remediation == "" {
		t.Error("the hint has to survive as Remediation")
	}

	h := find(t, fs, "origin.example:443/pq-preferred")
	if h.Status != engine.WARN {
		t.Errorf("handshake status = %s, want WARN", h.Status)
	}
}

// unreachable is not a grade, and it must not arrive as BAD: an endpoint nobody
// reached says nothing about post-quantum support.
func TestUnreachableIsAnError(t *testing.T) {
	c := check([]pqprobe.Report{{
		Target: "gone.example:443", Class: "unreachable", Worst: "ERROR",
		Findings: []pqprobe.Finding{
			{Check: "verdict", Target: "gone.example:443", Status: "ERROR",
				Message: "unreachable — nothing answered", Hint: "fix reachability first"},
		},
	}}, nil)

	f := c.Run(context.Background())[0]
	if f.Status != engine.ERROR {
		t.Errorf("status = %s, want ERROR", f.Status)
	}
}

// The numbers travel as numbers, here as everywhere: a dashboard must not parse
// a sentence to plot certificate days.
func TestValuesSurviveTheMapping(t *testing.T) {
	days := 21.0
	c := check([]pqprobe.Report{{
		Target: "h:443", Class: "pq-blind", Worst: "WARN",
		Findings: []pqprobe.Finding{
			{Check: "verdict", Target: "h:443", Status: "WARN", Message: "pq-blind"},
			{Check: "expiry", Target: "h:443", Status: "WARN", Message: "21 days",
				Value: &days, Unit: "days"},
		},
	}}, nil)

	e := find(t, c.Run(context.Background()), "h:443/expiry")
	if e.Value == nil || *e.Value != 21 || e.Unit != "days" {
		t.Errorf("value did not survive: %+v", e)
	}
}

// Something the caller got wrong — an unknown profile, no target — is one ERROR
// row per configured target rather than a silent empty run.
func TestAProbeErrorIsReportedNotSwallowed(t *testing.T) {
	c := check(nil, errors.New("unknown profile(s): [nonsense]"))
	fs := c.Run(context.Background())
	if len(fs) != 1 {
		t.Fatalf("got %d findings, want one per configured target: %+v", len(fs), fs)
	}
	if fs[0].Status != engine.ERROR {
		t.Errorf("status = %s, want ERROR", fs[0].Status)
	}
	if fs[0].Message == "" || fs[0].Target != "origin.example" {
		t.Errorf("finding = %+v", fs[0])
	}
}
