// Package redis implements a health check for Redis / Valkey using a minimal
// in-tree RESP client (no third-party dependency). It reads INFO and flags:
// unreachability, dataset still loading, memory near maxmemory, broken/lagging
// replication, and failed persistence (RDB/AOF). Read-only commands only.
package redis

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/inventory"
)

type Check struct {
	cfg engine.RedisConfig
	// fetchInfo is injectable for tests; defaults to a live RESP INFO.
	fetchInfo func(ctx context.Context, target string) (string, error)
}

func New(cfg engine.RedisConfig) *Check {
	c := &Check{cfg: cfg}
	c.fetchInfo = c.liveInfo
	return c
}

func (c *Check) Name() string { return "redis" }

// Targets resolves explicit targets plus inventory hosts to host:port pairs.
func (c *Check) Targets() ([]string, error) {
	var targets []string
	for _, t := range c.cfg.Targets {
		targets = append(targets, withDefaultPort(t, c.cfg.Port))
	}
	if c.cfg.AnsibleInventory != "" {
		hosts, err := inventory.LoadPath(c.cfg.AnsibleInventory)
		if err != nil {
			return targets, fmt.Errorf("inventory %s: %w", c.cfg.AnsibleInventory, err)
		}
		for _, h := range hosts {
			targets = append(targets, withDefaultPort(h.Address, c.cfg.Port))
		}
	}
	return targets, nil
}

func (c *Check) Run(ctx context.Context) []engine.Finding {
	targets, err := c.Targets()
	var findings []engine.Finding
	if err != nil {
		findings = append(findings, engine.Finding{Check: c.Name(), Target: c.cfg.AnsibleInventory, Status: engine.ERROR, Message: err.Error()})
	}
	perTarget := make([][]engine.Finding, len(targets))
	sem := make(chan struct{}, 16)
	done := make(chan int)
	for i, target := range targets {
		go func(i int, target string) {
			sem <- struct{}{}
			perTarget[i] = c.probe(ctx, target)
			<-sem
			done <- i
		}(i, target)
	}
	for range targets {
		<-done
	}
	for _, fs := range perTarget {
		findings = append(findings, fs...)
	}
	return findings
}

func (c *Check) probe(ctx context.Context, target string) []engine.Finding {
	raw, err := c.fetchInfo(ctx, target)
	if err != nil {
		return []engine.Finding{{Check: c.Name(), Target: target, Status: engine.ERROR, Message: fmt.Sprintf("not reachable: %v", err)}}
	}
	return c.evaluate(target, parseInfo(raw))
}

func (c *Check) evaluate(target string, info map[string]string) []engine.Finding {
	role := info["role"]
	if role == "" {
		role = "?"
	}
	var findings []engine.Finding

	// Reachability / role (WARN while the dataset is still loading).
	reach := engine.Finding{Check: c.Name(), Target: target, Status: engine.OK,
		Message: fmt.Sprintf("v%s (%s)", info["redis_version"], role)}
	if info["loading"] == "1" {
		reach.Status, reach.Message = engine.WARN, "dataset loading"
	}
	findings = append(findings, reach)

	findings = append(findings, c.memoryFinding(target, info))
	if role == "slave" || role == "replica" {
		findings = append(findings, c.replicationFinding(target, info))
	}
	findings = append(findings, c.persistenceFindings(target, info)...)
	return findings
}

func (c *Check) memoryFinding(target string, info map[string]string) engine.Finding {
	f := engine.Finding{Check: c.Name(), Target: target + " [memory]"}
	used := atoi(info["used_memory"])
	max := atoi(info["maxmemory"])
	if max <= 0 {
		f.Status, f.Message = engine.OK, fmt.Sprintf("%s used, no maxmemory", humanBytes(used))
		return f
	}
	pct := int(used * 100 / max)
	if c.cfg.MemWarnPct > 0 && pct >= c.cfg.MemWarnPct {
		f.Status = engine.WARN
	} else {
		f.Status = engine.OK
	}
	f.Message = fmt.Sprintf("%s/%s (%d%% of maxmemory)", humanBytes(used), humanBytes(max), pct)
	return f
}

func (c *Check) replicationFinding(target string, info map[string]string) engine.Finding {
	f := engine.Finding{Check: c.Name(), Target: target + " [replication]"}
	if info["master_link_status"] != "up" {
		f.Status, f.Message = engine.BAD, fmt.Sprintf("master link %q (not up)", info["master_link_status"])
		return f
	}
	lag := atoi(info["master_repl_offset"]) - atoi(info["slave_repl_offset"])
	if lag < 0 {
		lag = 0
	}
	switch {
	case c.cfg.LagCritBytes > 0 && lag >= c.cfg.LagCritBytes:
		f.Status, f.Message = engine.BAD, fmt.Sprintf("replica lag %s over critical threshold (%s)", humanBytes(lag), humanBytes(c.cfg.LagCritBytes))
	case c.cfg.LagWarnBytes > 0 && lag >= c.cfg.LagWarnBytes:
		f.Status, f.Message = engine.WARN, fmt.Sprintf("replica lag %s over threshold (%s)", humanBytes(lag), humanBytes(c.cfg.LagWarnBytes))
	default:
		f.Status, f.Message = engine.OK, fmt.Sprintf("link up, lag %s", humanBytes(lag))
	}
	return f
}

func (c *Check) persistenceFindings(target string, info map[string]string) []engine.Finding {
	var findings []engine.Finding
	if st := info["rdb_last_bgsave_status"]; st != "" && st != "ok" {
		findings = append(findings, engine.Finding{Check: c.Name(), Target: target + " [persistence]", Status: engine.WARN, Message: "last RDB bgsave failed: " + st})
	}
	if info["aof_enabled"] == "1" {
		if st := info["aof_last_write_status"]; st != "" && st != "ok" {
			findings = append(findings, engine.Finding{Check: c.Name(), Target: target + " [persistence]", Status: engine.WARN, Message: "last AOF write failed: " + st})
		}
	}
	return findings
}

// ---------- live RESP client ----------

func (c *Check) liveInfo(ctx context.Context, target string) (string, error) {
	var conn net.Conn
	var err error
	d := net.Dialer{}
	if c.cfg.TLS {
		conn, err = tls.DialWithDialer(&d, "tcp", target, &tls.Config{ServerName: hostOf(target)})
	} else {
		conn, err = d.DialContext(ctx, "tcp", target)
	}
	if err != nil {
		return "", err
	}
	defer conn.Close()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	r := bufio.NewReader(conn)

	if pw := os.Getenv(c.cfg.PasswordEnv); c.cfg.PasswordEnv != "" && pw != "" {
		args := []string{"AUTH", pw}
		if c.cfg.Username != "" {
			args = []string{"AUTH", c.cfg.Username, pw}
		}
		if _, err := command(conn, r, args...); err != nil {
			return "", fmt.Errorf("AUTH: %w", err)
		}
	}
	if _, err := command(conn, r, "PING"); err != nil {
		return "", err
	}
	return command(conn, r, "INFO")
}

// command writes a RESP array command and returns the reply's string value.
func command(conn net.Conn, r *bufio.Reader, args ...string) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "*%d\r\n", len(args))
	for _, a := range args {
		fmt.Fprintf(&b, "$%d\r\n%s\r\n", len(a), a)
	}
	if _, err := conn.Write([]byte(b.String())); err != nil {
		return "", err
	}
	return readReply(r)
}

func readReply(r *bufio.Reader) (string, error) {
	prefix, err := r.ReadByte()
	if err != nil {
		return "", err
	}
	line, err := readLine(r)
	if err != nil {
		return "", err
	}
	switch prefix {
	case '+', ':':
		return line, nil
	case '-':
		return "", fmt.Errorf("errore redis: %s", line)
	case '$':
		n, err := strconv.Atoi(line)
		if err != nil {
			return "", err
		}
		if n < 0 {
			return "", nil // nil bulk
		}
		buf := make([]byte, n+2) // payload + CRLF
		if _, err := readFull(r, buf); err != nil {
			return "", err
		}
		return string(buf[:n]), nil
	default:
		return "", fmt.Errorf("unexpected RESP prefix: %q", prefix)
	}
}

func readLine(r *bufio.Reader) (string, error) {
	s, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(s, "\r\n"), nil
}

func readFull(r *bufio.Reader, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := r.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// ---------- helpers ----------

// parseInfo turns an INFO payload into a flat key→value map (sections ignored).
func parseInfo(payload string) map[string]string {
	info := map[string]string{}
	for _, line := range strings.Split(payload, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if i := strings.IndexByte(line, ':'); i > 0 {
			info[line[:i]] = line[i+1:]
		}
	}
	return info
}

func atoi(s string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func hostOf(target string) string {
	if h, _, err := net.SplitHostPort(target); err == nil {
		return h
	}
	return target
}

func withDefaultPort(target string, port int) string {
	if strings.Contains(target, ":") {
		return target
	}
	return fmt.Sprintf("%s:%d", target, port)
}
