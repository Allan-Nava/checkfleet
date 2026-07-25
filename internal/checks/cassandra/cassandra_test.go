package cassandra

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// fakeCQL answers OPTIONS with SUPPORTED (advertising a CQL_VERSION) and STARTUP
// with the given opcode (opReady / opAuthenticate / opError).
func fakeCQL(t *testing.T, startupReply byte) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serve(conn, startupReply)
		}
	}()
	return ln.Addr().String()
}

func serve(conn net.Conn, startupReply byte) {
	defer conn.Close()
	for {
		hdr := make([]byte, 9)
		if _, err := io.ReadFull(conn, hdr); err != nil {
			return
		}
		n := binary.BigEndian.Uint32(hdr[5:9])
		body := make([]byte, n)
		if _, err := io.ReadFull(conn, body); err != nil {
			return
		}
		switch hdr[4] {
		case opOptions:
			// SUPPORTED: multimap {CQL_VERSION: ["3.4.5"]}
			b := []byte{0x00, 0x01} // 1 entry
			b = appendString(b, "CQL_VERSION")
			b = append(b, 0x00, 0x01) // list of 1
			b = appendString(b, "3.4.5")
			writeResp(conn, opSupported, b)
		case opStartup:
			switch startupReply {
			case opError:
				eb := []byte{0, 0, 0, 0} // int code
				eb = appendString(eb, "protocol error")
				writeResp(conn, opError, eb)
			default:
				writeResp(conn, startupReply, nil)
			}
		default:
			return
		}
	}
}

func writeResp(conn net.Conn, opcode byte, body []byte) {
	hdr := make([]byte, 9+len(body))
	hdr[0] = protoResponse
	binary.BigEndian.PutUint16(hdr[2:4], 1)
	hdr[4] = opcode
	binary.BigEndian.PutUint32(hdr[5:9], uint32(len(body)))
	copy(hdr[9:], body)
	_, _ = conn.Write(hdr)
}

func run(t *testing.T, target engine.CassandraTarget) engine.Finding {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return New(engine.CassandraConfig{}).probe(ctx, target)
}

func TestReadyIsOK(t *testing.T) {
	addr := fakeCQL(t, opReady)
	f := run(t, engine.CassandraTarget{Name: "cql", Address: addr})
	if f.Status != engine.OK || !strings.Contains(f.Message, "3.4.5") {
		t.Fatalf("READY should be OK with CQL version, got %s: %s", f.Status, f.Message)
	}
}

func TestAuthenticateIsOK(t *testing.T) {
	addr := fakeCQL(t, opAuthenticate)
	f := run(t, engine.CassandraTarget{Name: "cql", Address: addr})
	if f.Status != engine.OK || !strings.Contains(f.Message, "auth required") {
		t.Fatalf("AUTHENTICATE should be OK (auth required), got %s: %s", f.Status, f.Message)
	}
}

func TestStartupErrorIsBad(t *testing.T) {
	addr := fakeCQL(t, opError)
	f := run(t, engine.CassandraTarget{Name: "cql", Address: addr})
	if f.Status != engine.BAD || !strings.Contains(f.Message, "protocol error") {
		t.Fatalf("ERROR reply should be BAD with message, got %s: %s", f.Status, f.Message)
	}
}

func TestLatencyWarn(t *testing.T) {
	// Drive the clock so the handshake "takes" 50ms, over a 10ms threshold.
	addr := fakeCQL(t, opReady)
	c := New(engine.CassandraConfig{})
	calls := 0
	base := time.Unix(1000, 0)
	c.now = func() time.Time {
		calls++
		if calls == 1 {
			return base
		}
		return base.Add(50 * time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	f := c.probe(ctx, engine.CassandraTarget{Name: "cql", Address: addr, MaxLatencyMS: 10})
	if f.Status != engine.WARN || !strings.Contains(f.Message, "over 10ms") {
		t.Fatalf("50ms handshake over 10ms should WARN, got %s: %s", f.Status, f.Message)
	}
}

func TestConnectionRefusedIsError(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	ln.Close()
	f := run(t, engine.CassandraTarget{Name: "cql", Address: addr})
	if f.Status != engine.ERROR {
		t.Fatalf("refused connection should be ERROR, got %s: %s", f.Status, f.Message)
	}
}

func TestLatencyIsAMetric(t *testing.T) {
	addr := fakeCQL(t, opReady)
	f := run(t, engine.CassandraTarget{Name: "cql", Address: addr})
	if f.Value == nil || f.Unit != "ms" {
		t.Fatalf("handshake latency should be a ms metric, got value=%v unit=%q", f.Value, f.Unit)
	}
	if *f.Value < 0 {
		t.Errorf("latency must not be negative, got %v", *f.Value)
	}
}

// The cluster rollup is pure: feed it node findings and check the verdict.
func TestClusterRollup(t *testing.T) {
	node := func(s engine.Status) engine.Finding { return engine.Finding{Status: s} }
	cases := []struct {
		name    string
		expect  int
		nodes   []engine.Finding
		want    engine.Status
		wantUp  float64
		wantMsg string
	}{
		{
			name:  "all nodes up",
			nodes: []engine.Finding{node(engine.OK), node(engine.OK)},
			want:  engine.OK, wantUp: 2, wantMsg: "2/2 nodes accept CQL",
		},
		{
			// A slow node still took the handshake, so it counts as up.
			name:  "slow node still counts",
			nodes: []engine.Finding{node(engine.OK), node(engine.WARN)},
			want:  engine.OK, wantUp: 2,
		},
		{
			name:  "one node down without an expectation",
			nodes: []engine.Finding{node(engine.OK), node(engine.ERROR)},
			want:  engine.BAD, wantUp: 1, wantMsg: "expected 2",
		},
		{
			// expect_nodes met: degraded, not broken.
			name: "one node down but expectation met", expect: 2,
			nodes: []engine.Finding{node(engine.OK), node(engine.OK), node(engine.ERROR)},
			want:  engine.WARN, wantUp: 2, wantMsg: "met",
		},
		{
			name: "below expectation is bad", expect: 3,
			nodes: []engine.Finding{node(engine.OK), node(engine.BAD), node(engine.ERROR)},
			want:  engine.BAD, wantUp: 1, wantMsg: "expected 3",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := New(engine.CassandraConfig{ExpectNodes: tc.expect})
			f, ok := c.clusterFinding(tc.nodes)
			if !ok {
				t.Fatal("want a cluster finding, got none")
			}
			if f.Status != tc.want {
				t.Errorf("status: want %s, got %s (%s)", tc.want, f.Status, f.Message)
			}
			if f.Value == nil || *f.Value != tc.wantUp || f.Unit != "nodes" {
				t.Errorf("want %v nodes up as a metric, got value=%v unit=%q", tc.wantUp, f.Value, f.Unit)
			}
			if tc.wantMsg != "" && !strings.Contains(f.Message, tc.wantMsg) {
				t.Errorf("message %q should contain %q", f.Message, tc.wantMsg)
			}
		})
	}
}

// One node and no expectation: the rollup would just repeat the node finding.
func TestClusterRollupOmittedForSingleNode(t *testing.T) {
	c := New(engine.CassandraConfig{})
	if _, ok := c.clusterFinding([]engine.Finding{{Status: engine.OK}}); ok {
		t.Error("single node without expect_nodes should produce no cluster finding")
	}
	// ...unless an expectation was set explicitly.
	c = New(engine.CassandraConfig{ExpectNodes: 2})
	if _, ok := c.clusterFinding([]engine.Finding{{Status: engine.OK}}); !ok {
		t.Error("expect_nodes set: want a cluster finding")
	}
}

func TestRunReportsClusterState(t *testing.T) {
	up := fakeCQL(t, opReady)
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	down := ln.Addr().String()
	ln.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	findings := New(engine.CassandraConfig{Targets: []engine.CassandraTarget{
		{Name: "up", Address: up}, {Name: "down", Address: down},
	}, ExpectNodes: 1}).Run(ctx)

	if len(findings) != 3 {
		t.Fatalf("want 2 node findings + 1 cluster, got %d: %v", len(findings), findings)
	}
	cluster := findings[len(findings)-1]
	if cluster.Target != "cluster" || cluster.Status != engine.WARN {
		t.Fatalf("want a WARN cluster finding, got %s %s: %s", cluster.Target, cluster.Status, cluster.Message)
	}
	if !strings.Contains(cluster.Message, "1/2") {
		t.Errorf("message should report 1/2 nodes, got %q", cluster.Message)
	}
}

func TestDefaultPort(t *testing.T) {
	if got := withDefaultPort("node1"); got != "node1:9042" {
		t.Errorf("default port = %q, want node1:9042", got)
	}
}
