package redis

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// startFakeRedis serves a minimal RESP endpoint: AUTH/PING → +OK/+PONG, INFO →
// the given payload. If wantPass != "" it requires a matching AUTH.
func startFakeRedis(t *testing.T, info, wantPass string) string {
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
			go serve(conn, info, wantPass)
		}
	}()
	return ln.Addr().String()
}

func serve(conn net.Conn, info, wantPass string) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	authed := wantPass == ""
	for {
		args, err := readCommand(r)
		if err != nil {
			return
		}
		switch strings.ToUpper(args[0]) {
		case "AUTH":
			if args[len(args)-1] == wantPass {
				authed = true
				io.WriteString(conn, "+OK\r\n")
			} else {
				io.WriteString(conn, "-WRONGPASS\r\n")
			}
		case "PING":
			if !authed {
				io.WriteString(conn, "-NOAUTH Authentication required\r\n")
				continue
			}
			io.WriteString(conn, "+PONG\r\n")
		case "INFO":
			fmt.Fprintf(conn, "$%d\r\n%s\r\n", len(info), info)
		default:
			io.WriteString(conn, "-ERR unknown\r\n")
		}
	}
}

func readCommand(r *bufio.Reader) ([]string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimRight(line, "\r\n")
	if !strings.HasPrefix(line, "*") {
		return nil, fmt.Errorf("comando non-array: %q", line)
	}
	n, _ := strconv.Atoi(line[1:])
	args := make([]string, n)
	for i := 0; i < n; i++ {
		hdr, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		ln, _ := strconv.Atoi(strings.TrimRight(hdr, "\r\n")[1:])
		buf := make([]byte, ln+2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		args[i] = string(buf[:ln])
	}
	return args, nil
}

func run(t *testing.T, cfg engine.RedisConfig) map[string]engine.Finding {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	m := map[string]engine.Finding{}
	for _, f := range New(cfg).Run(ctx) {
		m[f.Target] = f
	}
	return m
}

func cfg(target string) engine.RedisConfig {
	return engine.RedisConfig{Targets: []string{target}, MemWarnPct: 80, LagWarnBytes: 16 << 20, LagCritBytes: 128 << 20}
}

const masterInfo = "# Server\r\nredis_version:7.2.4\r\n# Clients\r\nblocked_clients:0\r\n" +
	"# Memory\r\nused_memory:1048576\r\nmaxmemory:10485760\r\n" +
	"# Persistence\r\nloading:0\r\nrdb_last_bgsave_status:ok\r\naof_enabled:0\r\n" +
	"# Replication\r\nrole:master\r\nconnected_slaves:1\r\nmaster_repl_offset:5000\r\n"

func TestHealthyMaster(t *testing.T) {
	f := run(t, cfg(startFakeRedis(t, masterInfo, "")))
	addr := firstKey(f)
	if got := f[addr]; got.Status != engine.OK || !strings.Contains(got.Message, "master") {
		t.Errorf("reachability: atteso OK master, avuto %s (%s)", got.Status, got.Message)
	}
	if got := f[addr+" [memory]"]; got.Status != engine.OK {
		t.Errorf("memoria 10%%: atteso OK, avuto %s (%s)", got.Status, got.Message)
	}
}

func TestMemoryHighIsWarn(t *testing.T) {
	info := "# Server\r\nredis_version:7.2\r\n# Memory\r\nused_memory:9000000\r\nmaxmemory:10000000\r\n# Replication\r\nrole:master\r\n"
	f := run(t, cfg(startFakeRedis(t, info, "")))
	if got := f[firstKey(f)+" [memory]"]; got.Status != engine.WARN {
		t.Errorf("memoria 90%%: atteso WARN, avuto %s (%s)", got.Status, got.Message)
	}
}

func TestReplicaLinkDownIsBad(t *testing.T) {
	info := "redis_version:7.2\r\nrole:slave\r\nmaster_link_status:down\r\n"
	f := run(t, cfg(startFakeRedis(t, info, "")))
	if got := f[firstKey(f)+" [replication]"]; got.Status != engine.BAD {
		t.Errorf("link master down: atteso BAD, avuto %s (%s)", got.Status, got.Message)
	}
}

func TestReplicaLagIsBad(t *testing.T) {
	info := "redis_version:7.2\r\nrole:slave\r\nmaster_link_status:up\r\nmaster_repl_offset:300000000\r\nslave_repl_offset:100000000\r\n"
	f := run(t, cfg(startFakeRedis(t, info, "")))
	if got := f[firstKey(f)+" [replication]"]; got.Status != engine.BAD {
		t.Errorf("lag replica enorme: atteso BAD, avuto %s (%s)", got.Status, got.Message)
	}
}

func TestPersistenceFailIsWarn(t *testing.T) {
	info := "redis_version:7.2\r\nrole:master\r\nrdb_last_bgsave_status:err\r\n"
	f := run(t, cfg(startFakeRedis(t, info, "")))
	if got := f[firstKey(f)+" [persistence]"]; got.Status != engine.WARN {
		t.Errorf("bgsave fallito: atteso WARN, avuto %s (%s)", got.Status, got.Message)
	}
}

func TestLoadingIsWarn(t *testing.T) {
	info := "redis_version:7.2\r\nrole:master\r\nloading:1\r\n"
	f := run(t, cfg(startFakeRedis(t, info, "")))
	if got := f[firstKey(f)]; got.Status != engine.WARN || !strings.Contains(got.Message, "caricamento") {
		t.Errorf("loading: atteso WARN, avuto %s (%s)", got.Status, got.Message)
	}
}

func TestAuthFromEnv(t *testing.T) {
	t.Setenv("REDIS_PASS", "s3cr3t")
	c := cfg(startFakeRedis(t, masterInfo, "s3cr3t"))
	c.PasswordEnv = "REDIS_PASS"
	f := run(t, c)
	if got := f[firstKey(f)]; got.Status != engine.OK {
		t.Errorf("con auth corretta: atteso OK, avuto %s (%s)", got.Status, got.Message)
	}
}

func TestUnreachableIsError(t *testing.T) {
	f := run(t, cfg("127.0.0.1:1"))
	for _, v := range f {
		if v.Status != engine.ERROR {
			t.Errorf("irraggiungibile: atteso ERROR, avuto %s (%s)", v.Status, v.Message)
		}
	}
}

// firstKey returns the reachability target (the one without a " [" suffix).
func firstKey(f map[string]engine.Finding) string {
	for k := range f {
		if !strings.Contains(k, " [") {
			return k
		}
	}
	return ""
}
