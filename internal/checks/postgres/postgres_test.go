package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// fakeCollector returns canned metrics (or an error), no real database.
type fakeCollector struct {
	m   metrics
	err error
}

func (f fakeCollector) Collect(context.Context) (metrics, error) { return f.m, f.err }
func (f fakeCollector) Close(context.Context)                    {}

// checkWith builds a Check whose connect returns the given collector.
func checkWith(cfg engine.PostgresConfig, coll collector, connErr error) *Check {
	c := New(cfg)
	c.connect = func(context.Context, engine.PostgresTarget) (collector, error) {
		if connErr != nil {
			return nil, connErr
		}
		return coll, nil
	}
	return c
}

func defaults() engine.PostgresConfig {
	return engine.PostgresConfig{
		Targets:           []engine.PostgresTarget{{Name: "db1", DSN: "host=x"}},
		LagWarnBytes:      32 << 20,
		LagCritBytes:      128 << 20,
		ConnWarnPct:       80,
		WraparoundWarnAge: 1_500_000_000,
		WraparoundCritAge: 1_900_000_000,
		SlotWarnBytes:     512 << 20,
		SlotCritBytes:     2 << 30,
	}
}

func run(t *testing.T, c *Check) map[string]engine.Finding {
	t.Helper()
	m := map[string]engine.Finding{}
	for _, f := range c.Run(context.Background()) {
		m[f.Target] = f
	}
	return m
}

func TestHealthyPrimary(t *testing.T) {
	c := checkWith(defaults(), fakeCollector{m: metrics{
		InRecovery: false, WraparoundAge: 200_000_000, Connections: 10, MaxConnections: 100,
		Replicas: []replica{{Client: "10.0.0.2", State: "streaming", LagBytes: 1 << 20}},
	}}, nil)
	f := run(t, c)
	if got := f["db1"]; got.Status != engine.OK || !strings.Contains(got.Message, "primary") {
		t.Errorf("reachability: want OK primary, got %s (%s)", got.Status, got.Message)
	}
	for _, k := range []string{"db1 [wraparound]", "db1 [connections]", "db1 [repl:10.0.0.2]"} {
		if f[k].Status != engine.OK {
			t.Errorf("%s: want OK, got %s (%s)", k, f[k].Status, f[k].Message)
		}
	}
}

func TestWraparoundThresholds(t *testing.T) {
	for _, tc := range []struct {
		age  int64
		want engine.Status
	}{
		{200_000_000, engine.OK},
		{1_600_000_000, engine.WARN},
		{1_950_000_000, engine.BAD},
	} {
		c := checkWith(defaults(), fakeCollector{m: metrics{WraparoundAge: tc.age, MaxConnections: 100}}, nil)
		if got := run(t, c)["db1 [wraparound]"]; got.Status != tc.want {
			t.Errorf("age %d: want %s, got %s (%s)", tc.age, tc.want, got.Status, got.Message)
		}
	}
}

func TestConnectionsSaturation(t *testing.T) {
	c := checkWith(defaults(), fakeCollector{m: metrics{Connections: 85, MaxConnections: 100}}, nil)
	if got := run(t, c)["db1 [connections]"]; got.Status != engine.WARN || !strings.Contains(got.Message, "85%") {
		t.Errorf("85/100: want WARN 85%%, got %s (%s)", got.Status, got.Message)
	}
}

func TestInactiveSlotThresholds(t *testing.T) {
	c := checkWith(defaults(), fakeCollector{m: metrics{
		MaxConnections: 100,
		InactiveSlots: []slot{
			{Name: "small", RetainedBytes: 100 << 20}, // < warn 512MiB → WARN (inactive)
			{Name: "huge", RetainedBytes: 3 << 30},    // > crit 2GiB → BAD
		},
	}}, nil)
	f := run(t, c)
	if got := f["db1 [slot:small]"]; got.Status != engine.WARN {
		t.Errorf("small inactive slot: want WARN, got %s (%s)", got.Status, got.Message)
	}
	if got := f["db1 [slot:huge]"]; got.Status != engine.BAD {
		t.Errorf("huge slot: want BAD, got %s (%s)", got.Status, got.Message)
	}
}

func TestReplicaLagThresholds(t *testing.T) {
	c := checkWith(defaults(), fakeCollector{m: metrics{
		MaxConnections: 100,
		Replicas: []replica{
			{Client: "warn", State: "streaming", LagBytes: 64 << 20},
			{Client: "bad", State: "streaming", LagBytes: 256 << 20},
		},
	}}, nil)
	f := run(t, c)
	if f["db1 [repl:warn]"].Status != engine.WARN {
		t.Errorf("lag 64MiB: want WARN, got %+v", f["db1 [repl:warn]"])
	}
	if f["db1 [repl:bad]"].Status != engine.BAD {
		t.Errorf("lag 256MiB: want BAD, got %+v", f["db1 [repl:bad]"])
	}
}

func TestReplicaInRecoverySkipsReplicaChecks(t *testing.T) {
	c := checkWith(defaults(), fakeCollector{m: metrics{InRecovery: true, MaxConnections: 100}}, nil)
	f := run(t, c)
	if got := f["db1"]; !strings.Contains(got.Message, "replica") {
		t.Errorf("reachability on replica: want replica role, got %s", got.Message)
	}
	for k := range f {
		if strings.Contains(k, "[repl:") {
			t.Errorf("una replica non deve produrre finding pg_stat_replication: %s", k)
		}
	}
}

func TestConnectErrorIsError(t *testing.T) {
	c := checkWith(defaults(), nil, errors.New("connection refused"))
	if got := run(t, c)["db1"]; got.Status != engine.ERROR {
		t.Errorf("connect failed: want ERROR, got %s (%s)", got.Status, got.Message)
	}
}

func TestCollectErrorIsError(t *testing.T) {
	c := checkWith(defaults(), fakeCollector{err: errors.New("permission denied")}, nil)
	if got := run(t, c)["db1"]; got.Status != engine.ERROR || !strings.Contains(got.Message, "query") {
		t.Errorf("query failed: want ERROR, got %s (%s)", got.Status, got.Message)
	}
}
