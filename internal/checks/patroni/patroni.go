// Package patroni implements a health check for a Patroni-managed PostgreSQL
// cluster, reading the Patroni REST API (/cluster). It flags a missing or
// split leader, replicas in a bad state, replicas lagging beyond a threshold,
// and timeline divergence. It only reads the API; it never touches PostgreSQL.
package patroni

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/inventory"
)

type Check struct {
	cfg    engine.PatroniConfig
	client *http.Client
}

func New(cfg engine.PatroniConfig) *Check {
	return &Check{cfg: cfg, client: &http.Client{}}
}

func (c *Check) Name() string { return "patroni" }

type member struct {
	Name     string          `json:"name"`
	Role     string          `json:"role"`
	State    string          `json:"state"`
	Timeline int             `json:"timeline"`
	Lag      json.RawMessage `json:"lag"`
}

type cluster struct {
	Members []member `json:"members"`
	Scope   string   `json:"scope"`
}

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

	// Every endpoint proxies the same cluster view: use the first reachable one
	// for the member analysis, and report ERROR only for unreachable endpoints.
	var view *cluster
	results := c.fetchAll(ctx, targets)
	for i, target := range targets {
		if results[i].err != nil {
			findings = append(findings, engine.Finding{Check: c.Name(), Target: target, Status: engine.ERROR, Message: fmt.Sprintf("Patroni API not reachable: %v", results[i].err)})
			continue
		}
		if view == nil {
			view = results[i].cl
		}
	}
	if view != nil {
		findings = append(findings, c.analyze(*view)...)
	}
	return findings
}

type result struct {
	cl  *cluster
	err error
}

func (c *Check) fetchAll(ctx context.Context, targets []string) []result {
	results := make([]result, len(targets))
	sem := make(chan struct{}, 16)
	done := make(chan int)
	for i, target := range targets {
		go func(i int, target string) {
			sem <- struct{}{}
			results[i] = c.fetch(ctx, target)
			<-sem
			done <- i
		}(i, target)
	}
	for range targets {
		<-done
	}
	return results
}

func (c *Check) fetch(ctx context.Context, target string) result {
	url := c.scheme() + "://" + target + "/cluster"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return result{err: err}
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return result{err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return result{err: fmt.Errorf("HTTP %d", resp.StatusCode)}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return result{err: err}
	}
	var cl cluster
	if err := json.Unmarshal(body, &cl); err != nil {
		return result{err: err}
	}
	return result{cl: &cl}
}

func (c *Check) analyze(cl cluster) []engine.Finding {
	var findings []engine.Finding

	var leaders []member
	for _, m := range cl.Members {
		if m.Role == "leader" || m.Role == "standby_leader" {
			leaders = append(leaders, m)
		}
	}
	scope := cl.Scope
	if scope == "" {
		scope = "cluster"
	}
	switch len(leaders) {
	case 0:
		findings = append(findings, engine.Finding{Check: c.Name(), Target: scope, Status: engine.BAD, Message: "no leader in the cluster (failover in progress or quorum lost?)"})
	case 1:
		findings = append(findings, engine.Finding{Check: c.Name(), Target: scope, Status: engine.OK, Message: "leader: " + leaders[0].Name})
	default:
		findings = append(findings, engine.Finding{Check: c.Name(), Target: scope, Status: engine.WARN, Message: "more than one leader (split-brain?): " + memberNames(leaders)})
	}

	leaderTimeline := 0
	if len(leaders) == 1 {
		leaderTimeline = leaders[0].Timeline
	}
	for _, m := range cl.Members {
		findings = append(findings, c.memberFinding(m, leaderTimeline))
	}
	return findings
}

func (c *Check) memberFinding(m member, leaderTimeline int) engine.Finding {
	f := engine.Finding{Check: c.Name(), Target: m.Name}
	if m.Role == "leader" || m.Role == "standby_leader" {
		f.Status, f.Message = engine.OK, fmt.Sprintf("%s (timeline %d, %s)", m.Role, m.Timeline, m.State)
		return f
	}

	// Replica: state, then lag, then timeline — keep the worst.
	if s := replicaState(m.State); s != engine.OK {
		f.Status, f.Message = s, fmt.Sprintf("state %q", m.State)
		return f
	}
	if lag, ok := parseLag(m.Lag); ok {
		if c.cfg.LagCritBytes > 0 && lag >= c.cfg.LagCritBytes {
			f.Status, f.Message = engine.BAD, fmt.Sprintf("lag %s over critical threshold (%s)", humanBytes(lag), humanBytes(c.cfg.LagCritBytes))
			return f
		}
		if c.cfg.LagWarnBytes > 0 && lag >= c.cfg.LagWarnBytes {
			f.Status, f.Message = engine.WARN, fmt.Sprintf("lag %s over threshold (%s)", humanBytes(lag), humanBytes(c.cfg.LagWarnBytes))
			return f
		}
	}
	if leaderTimeline > 0 && m.Timeline > 0 && m.Timeline != leaderTimeline {
		f.Status, f.Message = engine.WARN, fmt.Sprintf("timeline %d differs from the leader (%d)", m.Timeline, leaderTimeline)
		return f
	}
	lagMsg := "unknown lag"
	if lag, ok := parseLag(m.Lag); ok {
		lagMsg = "lag " + humanBytes(lag)
	}
	f.Status, f.Message = engine.OK, fmt.Sprintf("%s (%s, %s)", m.Role, m.State, lagMsg)
	return f
}

// replicaState maps a Patroni member state to a status.
func replicaState(state string) engine.Status {
	switch state {
	case "running", "streaming":
		return engine.OK
	case "stopped", "start failed", "crashed", "restarting":
		return engine.BAD
	default:
		return engine.WARN
	}
}

// parseLag reads the Patroni "lag" field, which is an integer of bytes or the
// string "unknown" (or absent). Returns (bytes, known).
func parseLag(raw json.RawMessage) (int64, bool) {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return 0, false
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n, true
	}
	return 0, false
}

func memberNames(ms []member) string {
	names := make([]string, len(ms))
	for i, m := range ms {
		names[i] = m.Name
	}
	return strings.Join(names, ", ")
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

func (c *Check) scheme() string {
	if c.cfg.Scheme != "" {
		return c.cfg.Scheme
	}
	return "http"
}

func withDefaultPort(target string, port int) string {
	if strings.Contains(target, ":") {
		return target
	}
	return fmt.Sprintf("%s:%d", target, port)
}
