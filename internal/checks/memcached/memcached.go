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
	findings := make([]engine.Finding, len(c.cfg.Targets))
	sem := make(chan struct{}, 16)
	done := make(chan int)
	for i, t := range c.cfg.Targets {
		go func(i int, t string) {
			sem <- struct{}{}
			findings[i] = c.probe(ctx, withDefaultPort(t, c.cfg.Port))
			<-sem
			done <- i
		}(i, t)
	}
	for range c.cfg.Targets {
		<-done
	}
	return findings
}

func (c *Check) probe(ctx context.Context, target string) engine.Finding {
	f := engine.Finding{Check: c.Name(), Target: target}

	stats, err := c.stats(ctx, target)
	if err != nil {
		f.Status, f.Message = engine.ERROR, fmt.Sprintf("stats failed: %v", err)
		return f
	}

	version := stats["version"]
	conns := stats["curr_connections"]
	used, _ := strconv.ParseInt(stats["bytes"], 10, 64)
	limit, _ := strconv.ParseInt(stats["limit_maxbytes"], 10, 64)

	if limit > 0 {
		pct := int(used * 100 / limit)
		if c.cfg.MemWarnPct > 0 && pct >= c.cfg.MemWarnPct {
			f.Status = engine.WARN
			f.Message = fmt.Sprintf("memory %d%% of limit (%s of %s, over %d%%), memcached %s",
				pct, humanBytes(used), humanBytes(limit), c.cfg.MemWarnPct, version)
			return f
		}
		f.Status, f.Message = engine.OK, fmt.Sprintf("reachable, memcached %s, memory %d%%, %s conns", version, pct, conns)
		return f
	}
	f.Status, f.Message = engine.OK, fmt.Sprintf("reachable, memcached %s, %s conns", version, conns)
	return f
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
