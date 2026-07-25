// Package clickhouse implements a health check for ClickHouse over its HTTP
// interface: reachability (/ping), the query engine (SELECT version()), and —
// for replicated tables — read-only state and replication delay. Read-only,
// HTTP, zero third-party dependency.
package clickhouse

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

type Check struct {
	cfg engine.ClickHouseConfig
}

func New(cfg engine.ClickHouseConfig) *Check { return &Check{cfg: cfg} }

func (c *Check) Name() string { return "clickhouse" }

func (c *Check) Run(ctx context.Context) []engine.Finding {
	var findings []engine.Finding
	for _, t := range c.cfg.Targets {
		findings = append(findings, c.probe(ctx, t)...)
	}
	return findings
}

func (c *Check) probe(ctx context.Context, t engine.ClickHouseTarget) []engine.Finding {
	label := t.Name
	if label == "" {
		label = hostOf(t.URL)
	}
	client := c.clientFor(t)

	// /ping is a cheap liveness probe that returns "Ok.".
	if body, err := c.get(ctx, client, t, "/ping"); err != nil {
		return []engine.Finding{{Check: c.Name(), Target: label, Status: engine.ERROR,
			Message: fmt.Sprintf("ping failed: %v", err)}}
	} else if !strings.HasPrefix(strings.TrimSpace(body), "Ok") {
		return []engine.Finding{{Check: c.Name(), Target: label, Status: engine.BAD,
			Message: "ping did not return Ok"}}
	}

	version, err := c.query(ctx, client, t, "SELECT version()")
	if err != nil {
		return []engine.Finding{{Check: c.Name(), Target: label, Status: engine.ERROR,
			Message: fmt.Sprintf("query engine unreachable: %v", err)}}
	}

	findings := []engine.Finding{{Check: c.Name(), Target: label, Status: engine.OK,
		Message: fmt.Sprintf("reachable, ClickHouse %s", strings.TrimSpace(version))}}

	findings = append(findings, c.replicaFindings(ctx, client, t, label)...)
	return findings
}

// replicaFindings inspects system.replicas. Only tables with a problem produce a
// finding; a cluster with no replicated tables (or no access) adds nothing.
func (c *Check) replicaFindings(ctx context.Context, client *http.Client, t engine.ClickHouseTarget, label string) []engine.Finding {
	out, err := c.query(ctx, client, t,
		"SELECT database, table, is_readonly, absolute_delay FROM system.replicas FORMAT TSV")
	if err != nil {
		return nil
	}
	var findings []engine.Finding
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		cols := strings.Split(line, "\t")
		if len(cols) < 4 {
			continue
		}
		name := cols[0] + "." + cols[1]
		readonly := cols[2] == "1"
		delay, _ := strconv.ParseFloat(cols[3], 64)
		f := engine.Finding{Check: c.Name(), Target: label + "/" + name}
		f.Value, f.Unit = engine.Num(delay), "s" // replication delay
		switch {
		case readonly:
			f.Status, f.Message = engine.BAD, "replica is read-only (lost ZooKeeper/Keeper session?)"
		case c.cfg.DelayCritSeconds > 0 && delay >= float64(c.cfg.DelayCritSeconds):
			f.Status, f.Message = engine.BAD, fmt.Sprintf("replication delay %.0fs (over %ds)", delay, c.cfg.DelayCritSeconds)
		case c.cfg.DelayWarnSeconds > 0 && delay >= float64(c.cfg.DelayWarnSeconds):
			f.Status, f.Message = engine.WARN, fmt.Sprintf("replication delay %.0fs (over %ds)", delay, c.cfg.DelayWarnSeconds)
		default:
			continue // healthy replica — no finding, base OK covers it
		}
		findings = append(findings, f)
	}
	return findings
}

func (c *Check) get(ctx context.Context, client *http.Client, t engine.ClickHouseTarget, path string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(t.URL, "/")+path, nil)
	if err != nil {
		return "", err
	}
	return c.do(client, req, t)
}

func (c *Check) query(ctx context.Context, client *http.Client, t engine.ClickHouseTarget, sql string) (string, error) {
	u := strings.TrimRight(t.URL, "/") + "/?query=" + url.QueryEscape(sql)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	return c.do(client, req, t)
}

func (c *Check) do(client *http.Client, req *http.Request, t engine.ClickHouseTarget) (string, error) {
	if t.Username != "" {
		req.SetBasicAuth(t.Username, os.Getenv(t.PasswordEnv))
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return string(body), nil
}

func (c *Check) clientFor(t engine.ClickHouseTarget) *http.Client {
	if t.Insecure {
		return &http.Client{Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}}
	}
	return &http.Client{}
}

func hostOf(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return u.Host
	}
	return raw
}
