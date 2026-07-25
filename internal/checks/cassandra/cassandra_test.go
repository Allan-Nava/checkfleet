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

func TestDefaultPort(t *testing.T) {
	if got := withDefaultPort("node1"); got != "node1:9042" {
		t.Errorf("default port = %q, want node1:9042", got)
	}
}
