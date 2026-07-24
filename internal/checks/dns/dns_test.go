package dns

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// fakeDNS maps resolver→name→records (and optionally errors), no network.
type fakeDNS struct {
	byResolver map[string]map[string][]record
	errs       map[string]error
}

func (f fakeDNS) query(_ context.Context, resolver, name string, _ uint16) ([]record, error) {
	if err, ok := f.errs[resolver]; ok {
		return nil, err
	}
	return f.byResolver[resolver][name], nil
}

func checkWith(cfg engine.DNSConfig, f fakeDNS) *Check {
	c := New(cfg)
	c.query = f.query
	c.systemResolvers = func() []string { return nil }
	return c
}

func run(t *testing.T, c *Check) map[string]engine.Finding {
	t.Helper()
	m := map[string]engine.Finding{}
	for _, x := range c.Run(context.Background()) {
		m[x.Target] = x
	}
	return m
}

func aRec(ip string, ttl uint32) record { return record{Type: "A", Value: ip, TTL: ttl} }

func TestResolvesAndExpectMatch(t *testing.T) {
	cfg := engine.DNSConfig{
		Resolvers: []string{"10.0.0.53"},
		Targets:   []engine.DNSTarget{{Name: "example.com", Type: "A", Expect: []string{"1.2.3.4"}}},
	}
	f := fakeDNS{byResolver: map[string]map[string][]record{
		"10.0.0.53:53": {"example.com": {aRec("1.2.3.4", 300)}},
	}}
	if got := run(t, checkWith(cfg, f))["example.com/A"]; got.Status != engine.OK {
		t.Errorf("expected match: want OK, got %s (%s)", got.Status, got.Message)
	}
}

func TestDriftIsBad(t *testing.T) {
	cfg := engine.DNSConfig{
		Resolvers: []string{"10.0.0.53"},
		Targets:   []engine.DNSTarget{{Name: "example.com", Expect: []string{"1.2.3.4"}}},
	}
	f := fakeDNS{byResolver: map[string]map[string][]record{
		"10.0.0.53:53": {"example.com": {aRec("9.9.9.9", 300)}},
	}}
	if got := run(t, checkWith(cfg, f))["example.com/A"]; got.Status != engine.BAD || !strings.Contains(got.Message, "drift") {
		t.Errorf("drift: want BAD, got %s (%s)", got.Status, got.Message)
	}
}

func TestNoRecordIsBad(t *testing.T) {
	cfg := engine.DNSConfig{Resolvers: []string{"r"}, Targets: []engine.DNSTarget{{Name: "empty.example", Type: "A"}}}
	f := fakeDNS{byResolver: map[string]map[string][]record{"r:53": {"empty.example": nil}}}
	if got := run(t, checkWith(cfg, f))["empty.example/A"]; got.Status != engine.BAD {
		t.Errorf("no record: want BAD, got %s (%s)", got.Status, got.Message)
	}
}

func TestResolverDivergenceIsWarn(t *testing.T) {
	cfg := engine.DNSConfig{
		Resolvers: []string{"r1", "r2"},
		Targets:   []engine.DNSTarget{{Name: "example.com", Type: "A"}},
	}
	f := fakeDNS{byResolver: map[string]map[string][]record{
		"r1:53": {"example.com": {aRec("1.2.3.4", 300)}},
		"r2:53": {"example.com": {aRec("5.6.7.8", 300)}},
	}}
	if got := run(t, checkWith(cfg, f))["example.com/A [consistency]"]; got.Status != engine.WARN {
		t.Errorf("resolver divergence: want WARN, got %s (%s)", got.Status, got.Message)
	}
}

func TestSOASerialDivergenceIsWarn(t *testing.T) {
	cfg := engine.DNSConfig{
		Resolvers: []string{"r1", "r2"},
		Targets:   []engine.DNSTarget{{Name: "example.com", Type: "SOA"}},
	}
	f := fakeDNS{byResolver: map[string]map[string][]record{
		"r1:53": {"example.com": {{Type: "SOA", Value: "2026072401", TTL: 3600}}},
		"r2:53": {"example.com": {{Type: "SOA", Value: "2026072399", TTL: 3600}}},
	}}
	if got := run(t, checkWith(cfg, f))["example.com/SOA [consistency]"]; got.Status != engine.WARN {
		t.Errorf("SOA serial divergence: want WARN, got %s (%s)", got.Status, got.Message)
	}
}

func TestLowTTLIsWarn(t *testing.T) {
	cfg := engine.DNSConfig{
		Resolvers:     []string{"r"},
		MinTTLSeconds: 60,
		Targets:       []engine.DNSTarget{{Name: "example.com", Type: "A"}},
	}
	f := fakeDNS{byResolver: map[string]map[string][]record{"r:53": {"example.com": {aRec("1.2.3.4", 10)}}}}
	if got := run(t, checkWith(cfg, f))["example.com/A [ttl]"]; got.Status != engine.WARN {
		t.Errorf("low TTL: want WARN, got %s (%s)", got.Status, got.Message)
	}
}

func TestAllResolversFailIsError(t *testing.T) {
	cfg := engine.DNSConfig{Resolvers: []string{"r"}, Targets: []engine.DNSTarget{{Name: "x.example"}}}
	f := fakeDNS{errs: map[string]error{"r:53": errors.New("timeout")}}
	if got := run(t, checkWith(cfg, f))["x.example/A"]; got.Status != engine.ERROR {
		t.Errorf("all failed: want ERROR, got %s (%s)", got.Status, got.Message)
	}
}

func TestPartialResolverFailureIsWarn(t *testing.T) {
	cfg := engine.DNSConfig{
		Resolvers: []string{"ok", "down"},
		Targets:   []engine.DNSTarget{{Name: "example.com", Type: "A"}},
	}
	f := fakeDNS{
		byResolver: map[string]map[string][]record{"ok:53": {"example.com": {aRec("1.2.3.4", 300)}}},
		errs:       map[string]error{"down:53": errors.New("refused")},
	}
	res := run(t, checkWith(cfg, f))
	if got := res["example.com/A"]; got.Status != engine.OK {
		t.Errorf("one resolver ok: want OK resolution, got %s", got.Status)
	}
	if got := res["example.com/A [consistency]"]; got.Status != engine.WARN || !strings.Contains(got.Message, "did not respond") {
		t.Errorf("partial failure: want WARN, got %s (%s)", got.Status, got.Message)
	}
}

func TestUnsupportedTypeIsError(t *testing.T) {
	cfg := engine.DNSConfig{Resolvers: []string{"r"}, Targets: []engine.DNSTarget{{Name: "x", Type: "MX"}}}
	if got := run(t, checkWith(cfg, fakeDNS{}))["x/MX"]; got.Status != engine.ERROR {
		t.Errorf("unsupported type: want ERROR, got %s (%s)", got.Status, got.Message)
	}
}
