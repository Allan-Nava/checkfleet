package memcached

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// fakeMemcached serves one STATS response then closes.
func fakeMemcached(t *testing.T, stats map[string]string) string {
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
			go func(c net.Conn) {
				defer c.Close()
				sc := bufio.NewScanner(c)
				for sc.Scan() {
					cmd := strings.TrimSpace(sc.Text())
					if strings.HasPrefix(cmd, "stats") {
						for k, v := range stats {
							_, _ = c.Write([]byte("STAT " + k + " " + v + "\r\n"))
						}
						_, _ = c.Write([]byte("END\r\n"))
					} else if strings.HasPrefix(cmd, "quit") {
						return
					}
				}
			}(conn)
		}
	}()
	return ln.Addr().String()
}

func run(t *testing.T, cfg engine.MemcachedConfig, target string) engine.Finding {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return New(cfg).probe(ctx, target)
}

func TestReachableOK(t *testing.T) {
	addr := fakeMemcached(t, map[string]string{
		"version": "1.6.21", "curr_connections": "5", "bytes": "1000", "limit_maxbytes": "10000",
	})
	f := run(t, engine.MemcachedConfig{MemWarnPct: 90}, addr)
	if f.Status != engine.OK || !strings.Contains(f.Message, "1.6.21") {
		t.Fatalf("want OK with version, got %s: %s", f.Status, f.Message)
	}
}

func TestMemorySaturationWarns(t *testing.T) {
	addr := fakeMemcached(t, map[string]string{
		"version": "1.6", "bytes": "9500", "limit_maxbytes": "10000",
	})
	f := run(t, engine.MemcachedConfig{MemWarnPct: 90}, addr)
	if f.Status != engine.WARN || !strings.Contains(f.Message, "%") {
		t.Fatalf("95%% memory should WARN, got %s: %s", f.Status, f.Message)
	}
}

func TestUnreachableIsError(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	ln.Close()
	f := run(t, engine.MemcachedConfig{MemWarnPct: 90}, addr)
	if f.Status != engine.ERROR {
		t.Fatalf("unreachable should be ERROR, got %s: %s", f.Status, f.Message)
	}
}

func TestDefaultPort(t *testing.T) {
	if got := withDefaultPort("cache", 0); got != "cache:11211" {
		t.Errorf("default port = %q, want cache:11211", got)
	}
	if got := withDefaultPort("cache:9999", 11211); got != "cache:9999" {
		t.Errorf("explicit port must be kept, got %q", got)
	}
}
