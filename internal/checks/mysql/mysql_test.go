package mysql

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

type fakeCollector struct {
	m   metrics
	err error
}

func (f fakeCollector) Collect(context.Context) (metrics, error) { return f.m, f.err }
func (f fakeCollector) Close()                                   {}

func checkWith(cfg engine.MySQLConfig, m metrics, connErr, collErr error) []engine.Finding {
	c := New(cfg)
	c.connect = func(context.Context, engine.MySQLTarget) (collector, error) {
		if connErr != nil {
			return nil, connErr
		}
		return fakeCollector{m: m, err: collErr}, nil
	}
	return c.probe(context.Background(), engine.MySQLTarget{Name: "db", DSN: "x"})
}

func defaults() engine.MySQLConfig {
	return engine.MySQLConfig{ConnWarnPct: 80, LagWarnSeconds: 10, LagCritSeconds: 60}
}

func findingFor(fs []engine.Finding, target string) (engine.Finding, bool) {
	for _, f := range fs {
		if f.Target == target {
			return f, true
		}
	}
	return engine.Finding{}, false
}

func TestStandalonePrimaryOK(t *testing.T) {
	fs := checkWith(defaults(), metrics{Version: "8.0.36", Connections: 10, MaxConnections: 151}, nil, nil)
	base, _ := findingFor(fs, "db")
	if base.Status != engine.OK {
		t.Fatalf("want OK, got %s: %s", base.Status, base.Message)
	}
	if _, ok := findingFor(fs, "db/replication"); ok {
		t.Error("a non-replica should have no replication finding")
	}
}

func TestConnectionSaturation(t *testing.T) {
	fs := checkWith(defaults(), metrics{Connections: 130, MaxConnections: 151}, nil, nil)
	conn, _ := findingFor(fs, "db/connections")
	if conn.Status != engine.WARN {
		t.Fatalf("130/151 should WARN, got %s: %s", conn.Status, conn.Message)
	}
}

func TestReadOnlyRoleReported(t *testing.T) {
	fs := checkWith(defaults(), metrics{Version: "8.0", ReadOnly: true, MaxConnections: 100}, nil, nil)
	base, _ := findingFor(fs, "db")
	if base.Status != engine.OK || !strings.Contains(base.Message, "read-only") {
		t.Errorf("read-only role should be reported: %s", base.Message)
	}
}

func TestReplicaHealthyAndLag(t *testing.T) {
	mk := func(sec int64) metrics {
		return metrics{MaxConnections: 100, Replica: &replicaStatus{
			IORunning: true, SQLRunning: true, Replicating: true, SecondsBehind: sec}}
	}
	cases := []struct {
		sec  int64
		want engine.Status
	}{{2, engine.OK}, {20, engine.WARN}, {90, engine.BAD}}
	for _, tc := range cases {
		fs := checkWith(defaults(), mk(tc.sec), nil, nil)
		f, _ := findingFor(fs, "db/replication")
		if f.Status != tc.want {
			t.Errorf("lag %ds: want %s got %s (%s)", tc.sec, tc.want, f.Status, f.Message)
		}
	}
}

func TestReplicationStoppedIsBad(t *testing.T) {
	fs := checkWith(defaults(), metrics{MaxConnections: 100,
		Replica: &replicaStatus{IORunning: false, SQLRunning: true, Replicating: true}}, nil, nil)
	f, _ := findingFor(fs, "db/replication")
	if f.Status != engine.BAD || !strings.Contains(f.Message, "stopped") {
		t.Fatalf("stopped IO thread should be BAD, got %s: %s", f.Status, f.Message)
	}
}

func TestReplicaNotReplicatingIsBad(t *testing.T) {
	fs := checkWith(defaults(), metrics{MaxConnections: 100,
		Replica: &replicaStatus{IORunning: true, SQLRunning: true, Replicating: false}}, nil, nil)
	f, _ := findingFor(fs, "db/replication")
	if f.Status != engine.BAD || !strings.Contains(f.Message, "NULL") {
		t.Fatalf("NULL Seconds_Behind should be BAD, got %s: %s", f.Status, f.Message)
	}
}

func TestConnectAndCollectErrors(t *testing.T) {
	if fs := checkWith(defaults(), metrics{}, errors.New("refused"), nil); fs[0].Status != engine.ERROR {
		t.Errorf("connect error should be ERROR, got %s", fs[0].Status)
	}
	if fs := checkWith(defaults(), metrics{}, nil, errors.New("denied")); fs[0].Status != engine.ERROR {
		t.Errorf("collect error should be ERROR, got %s", fs[0].Status)
	}
}