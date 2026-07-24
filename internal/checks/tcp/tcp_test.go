package tcp

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// startServer accepts connections and writes banner (if non-empty) then closes.
func startServer(t *testing.T, banner string) string {
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
			if banner != "" {
				_, _ = conn.Write([]byte(banner))
			}
			conn.Close()
		}
	}()
	return ln.Addr().String()
}

func run(t *testing.T, targets ...engine.TCPTarget) map[string]engine.Finding {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	m := map[string]engine.Finding{}
	for _, f := range New(engine.TCPConfig{Targets: targets}).Run(ctx) {
		m[f.Target] = f
	}
	return m
}

func TestConnectOK(t *testing.T) {
	addr := startServer(t, "")
	if got := run(t, engine.TCPTarget{Address: addr})[addr]; got.Status != engine.OK {
		t.Errorf("connect ok: want OK, got %s (%s)", got.Status, got.Message)
	}
}

func TestBannerMatchAndMismatch(t *testing.T) {
	addr := startServer(t, "SSH-2.0-OpenSSH_9.6\r\n")
	if got := run(t, engine.TCPTarget{Name: "ssh", Address: addr, ExpectBanner: "SSH-2.0"})["ssh"]; got.Status != engine.OK {
		t.Errorf("banner match: want OK, got %s (%s)", got.Status, got.Message)
	}
	if got := run(t, engine.TCPTarget{Name: "ssh", Address: addr, ExpectBanner: "FTP"})["ssh"]; got.Status != engine.BAD {
		t.Errorf("banner mismatch: want BAD, got %s (%s)", got.Status, got.Message)
	}
}

func TestUnreachableIsError(t *testing.T) {
	if got := run(t, engine.TCPTarget{Address: "127.0.0.1:1"})["127.0.0.1:1"]; got.Status != engine.ERROR {
		t.Errorf("unreachable: want ERROR, got %s (%s)", got.Status, got.Message)
	}
}

func TestLatencyWarn(t *testing.T) {
	// now advances by 50ms between start and end → latency 50ms > max 10ms.
	addr := startServer(t, "")
	c := New(engine.TCPConfig{Targets: []engine.TCPTarget{{Address: addr, MaxLatencyMS: 10}}})
	calls := 0
	base := time.Unix(0, 0)
	c.now = func() time.Time {
		calls++
		if calls == 1 {
			return base
		}
		return base.Add(50 * time.Millisecond)
	}
	f := c.Run(context.Background())
	if f[0].Status != engine.WARN {
		t.Errorf("high latency: want WARN, got %s (%s)", f[0].Status, f[0].Message)
	}
}
