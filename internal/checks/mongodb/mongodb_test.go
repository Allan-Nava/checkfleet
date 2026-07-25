package mongodb

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// fakeCollector returns canned metrics (or an error), no real MongoDB.
type fakeCollector struct {
	m   metrics
	err error
}

func (f fakeCollector) Collect(context.Context) (metrics, error) { return f.m, f.err }
func (f fakeCollector) Close(context.Context)                    {}

func checkWith(cfg engine.MongoDBConfig, m metrics, connErr, collErr error) []engine.Finding {
	c := New(cfg)
	c.connect = func(context.Context, engine.MongoDBTarget) (collector, error) {
		if connErr != nil {
			return nil, connErr
		}
		return fakeCollector{m: m, err: collErr}, nil
	}
	return c.probe(context.Background(), engine.MongoDBTarget{Name: "db", URI: "mongodb://x:27017"})
}

func defaults() engine.MongoDBConfig {
	return engine.MongoDBConfig{ConnWarnPct: 80, LagWarnSeconds: 10, LagCritSeconds: 60}
}

func findingFor(fs []engine.Finding, target string) (engine.Finding, bool) {
	for _, f := range fs {
		if f.Target == target {
			return f, true
		}
	}
	return engine.Finding{}, false
}

func TestStandaloneReachable(t *testing.T) {
	fs := checkWith(defaults(), metrics{Version: "7.0.5", Standalone: true, Connections: 5, Available: 995}, nil, nil)
	base, _ := findingFor(fs, "db")
	if base.Status != engine.OK {
		t.Fatalf("standalone should be OK, got %s: %s", base.Status, base.Message)
	}
	// No replica-set findings for a standalone.
	if _, ok := findingFor(fs, "db/primary"); ok {
		t.Error("standalone should not produce replica findings")
	}
}

func TestConnectionSaturation(t *testing.T) {
	fs := checkWith(defaults(), metrics{Standalone: true, Connections: 85, Available: 15}, nil, nil)
	conn, _ := findingFor(fs, "db/connections")
	if conn.Status != engine.WARN {
		t.Fatalf("85%% connections should WARN, got %s: %s", conn.Status, conn.Message)
	}
}

func healthyRS() metrics {
	now := time.Now()
	return metrics{
		Version: "7.0.5", ReplSet: "rs0", Connections: 3, Available: 997,
		Members: []member{
			{Name: "m1:27017", Health: 1, StateStr: "PRIMARY", Optime: now},
			{Name: "m2:27017", Health: 1, StateStr: "SECONDARY", Optime: now.Add(-2 * time.Second)},
			{Name: "m3:27017", Health: 1, StateStr: "SECONDARY", Optime: now.Add(-1 * time.Second)},
		},
	}
}

func TestHealthyReplicaSet(t *testing.T) {
	fs := checkWith(defaults(), healthyRS(), nil, nil)
	for _, target := range []string{"db", "db/connections", "db/m1:27017", "db/m2:27017", "db/m3:27017"} {
		f, ok := findingFor(fs, target)
		if !ok {
			t.Fatalf("missing finding for %s", target)
		}
		if f.Status != engine.OK {
			t.Errorf("%s should be OK, got %s: %s", target, f.Status, f.Message)
		}
	}
	if _, ok := findingFor(fs, "db/primary"); ok {
		t.Error("healthy RS with a primary should not emit a db/primary finding")
	}
}

func TestNoPrimaryIsBad(t *testing.T) {
	m := healthyRS()
	m.Members[0].StateStr = "SECONDARY" // demote the primary
	fs := checkWith(defaults(), m, nil, nil)
	p, ok := findingFor(fs, "db/primary")
	if !ok || p.Status != engine.BAD {
		t.Fatalf("want BAD db/primary when no primary, got %+v", p)
	}
}

func TestUnhealthyMemberIsBad(t *testing.T) {
	m := healthyRS()
	m.Members[2].Health = 0
	fs := checkWith(defaults(), m, nil, nil)
	f, _ := findingFor(fs, "db/m3:27017")
	if f.Status != engine.BAD {
		t.Fatalf("health 0 member should be BAD, got %s: %s", f.Status, f.Message)
	}
}

func TestSecondaryLagThresholds(t *testing.T) {
	now := time.Now()
	mk := func(lag time.Duration) metrics {
		return metrics{
			ReplSet: "rs0", Connections: 1, Available: 99,
			Members: []member{
				{Name: "p:27017", Health: 1, StateStr: "PRIMARY", Optime: now},
				{Name: "s:27017", Health: 1, StateStr: "SECONDARY", Optime: now.Add(-lag)},
			},
		}
	}
	cases := []struct {
		lag  time.Duration
		want engine.Status
	}{
		{2 * time.Second, engine.OK},
		{20 * time.Second, engine.WARN},
		{90 * time.Second, engine.BAD},
	}
	for _, tc := range cases {
		fs := checkWith(defaults(), mk(tc.lag), nil, nil)
		f, _ := findingFor(fs, "db/s:27017")
		if f.Status != tc.want {
			t.Errorf("lag %s: want %s, got %s (%s)", tc.lag, tc.want, f.Status, f.Message)
		}
	}
}

func TestConnectErrorIsError(t *testing.T) {
	fs := checkWith(defaults(), metrics{}, errors.New("connection refused"), nil)
	base, _ := findingFor(fs, "db")
	if base.Status != engine.ERROR {
		t.Fatalf("connect error should be ERROR, got %s", base.Status)
	}
}

func TestCollectErrorIsError(t *testing.T) {
	fs := checkWith(defaults(), metrics{}, nil, errors.New("unauthorized"))
	base, _ := findingFor(fs, "db")
	if base.Status != engine.ERROR {
		t.Fatalf("collect error should be ERROR, got %s", base.Status)
	}
}

func TestHostOf(t *testing.T) {
	cases := map[string]string{
		"mongodb://h1:27017,h2:27017/admin": "h1:27017,h2:27017",
		"mongodb://user:pw@h:27017/?x=1":    "h:27017",
		"mongodb+srv://cluster.example.com":  "cluster.example.com",
	}
	for in, want := range cases {
		if got := hostOf(in); got != want {
			t.Errorf("hostOf(%q) = %q, want %q", in, got, want)
		}
	}
}
