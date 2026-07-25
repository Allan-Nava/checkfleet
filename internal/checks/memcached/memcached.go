// Package memcached implements a health check for memcached over its text
// protocol: reachability via STATS, memory usage vs limit_maxbytes, and basic
// counters. Zero third-party dependency.
package memcached

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

type Check struct {
	cfg engine.MemcachedConfig
}

func New(cfg engine.MemcachedConfig) *Check { return &Check{cfg: cfg} }

func (c *Check) Name() string { return "memcached" }

func (c *Check) Run(ctx context.Context) []engine.Finding {
	perTarget := make([][]engine.Finding, len(c.cfg.Targets))
	sem := make(chan struct{}, 16)
	done := make(chan int)
	for i, t := range c.cfg.Targets {
		go func(i int, t string) {
			sem <- struct{}{}
			perTarget[i] = c.probe(ctx, withDefaultPort(t, c.cfg.Port))
			<-sem
			done <- i
		}(i, t)
	}
	for range c.cfg.Targets {
		<-done
	}
	var findings []engine.Finding
	for _, fs := range perTarget {
		findings = append(findings, fs...)
	}
	return findings
}

func (c *Check) probe(ctx context.Context, target string) []engine.Finding {
	stats, err := c.stats(ctx, target)
	if err != nil {
		return []engine.Finding{{Check: c.Name(), Target: target, Status: engine.ERROR,
			Message: fmt.Sprintf("stats failed: %v", err)}}
	}
	findings := []engine.Finding{c.memoryFinding(target, stats)}
	if f, ok := c.evictionsFinding(target, stats); ok {
		findings = append(findings, f)
	}
	return findings
}

// memoryFinding reports reachability plus memory usage against limit_maxbytes;
// the percentage doubles as the target's numeric metric.
func (c *Check) memoryFinding(target string, stats map[string]string) engine.Finding {
	f := engine.Finding{Check: c.Name(), Target: target}
	version := stats["version"]
	conns := stats["curr_connections"]
	used := atoi(stats["bytes"])
	limit := atoi(stats["limit_maxbytes"])

	if limit <= 0 {
		f.Status, f.Message = engine.OK, fmt.Sprintf("reachable, memcached %s, %s conns", version, conns)
		return f
	}
	pct := int(used * 100 / limit)
	f.Value, f.Unit = engine.Num(float64(pct)), "%"
	if c.cfg.MemWarnPct > 0 && pct >= c.cfg.MemWarnPct {
		f.Status = engine.WARN
		f.Message = fmt.Sprintf("memory %d%% of limit (%s of %s, over %d%%), memcached %s",
			pct, humanBytes(used), humanBytes(limit), c.cfg.MemWarnPct, version)
		return f
	}
	f.Status, f.Message = engine.OK, fmt.Sprintf("reachable, memcached %s, memory %d%%, %s conns", version, pct, conns)
	return f
}

// evictionsFinding reports the eviction counter, which memcached exposes only
// as a total since startup. There is no rate to threshold on, so the count is
// published as a metric (charted over time from the history) and only WARNs
// against an explicit evictions_warn. Reported only when the server sends it.
func (c *Check) evictionsFinding(target string, stats map[string]string) (engine.Finding, bool) {
	raw, ok := stats["evictions"]
	if !ok {
		return engine.Finding{}, false
	}
	n := atoi(raw)
	f := engine.Finding{Check: c.Name(), Target: target + " [evictions]",
		Value: engine.Num(float64(n)), Unit: "evictions", Status: engine.OK,
		Message: fmt.Sprintf("%d evictions since start", n)}
	if c.cfg.EvictionsWarn > 0 && n >= c.cfg.EvictionsWarn {
		f.Status = engine.WARN
		f.Message = fmt.Sprintf("%d evictions since start (over %d)", n, c.cfg.EvictionsWarn)
	}
	return f, true
}

// stats opens a connection, runs STATS, and returns the name→value map.
func (c *Check) stats(ctx context.Context, target string) (map[string]string, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", target)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	} else {
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	}

	if _, err := fmt.Fprint(conn, "stats\r\n"); err != nil {
		return nil, err
	}
	out := map[string]string{}
	sc := bufio.NewScanner(conn)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if line == "END" {
			break
		}
		// "STAT <name> <value>"
		parts := strings.SplitN(line, " ", 3)
		if len(parts) == 3 && parts[0] == "STAT" {
			out[parts[1]] = parts[2]
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	_, _ = fmt.Fprint(conn, "quit\r\n")
	if len(out) == 0 {
		return nil, fmt.Errorf("no stats returned")
	}
	return out, nil
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

func withDefaultPort(target string, port int) string {
	if strings.Contains(target, ":") {
		return target
	}
	if port <= 0 {
		port = 11211
	}
	return fmt.Sprintf("%s:%d", target, port)
}
