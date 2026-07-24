// Package consul implements a health check for a Consul cluster via its HTTP
// API: a raft leader is elected, the raft peer count meets expectations,
// service/node health checks are not critical/warning, and required KV keys
// exist. Read-only; it never writes to Consul.
package consul

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/inventory"
)

type Check struct {
	cfg    engine.ConsulConfig
	client *http.Client
}

func New(cfg engine.ConsulConfig) *Check {
	return &Check{cfg: cfg, client: &http.Client{}}
}

func (c *Check) Name() string { return "consul" }

type healthCheck struct {
	Node        string `json:"Node"`
	CheckID     string `json:"CheckID"`
	Name        string `json:"Name"`
	Status      string `json:"Status"`
	ServiceName string `json:"ServiceName"`
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

	// Any agent answers cluster-wide status: find the first reachable one and
	// run the analysis against it; report ERROR for unreachable endpoints.
	base := ""
	for _, target := range targets {
		if _, _, err := c.get(ctx, target, "/v1/status/leader"); err != nil {
			findings = append(findings, engine.Finding{Check: c.Name(), Target: target, Status: engine.ERROR, Message: fmt.Sprintf("Consul API not reachable: %v", err)})
			continue
		}
		if base == "" {
			base = target
		}
	}
	if base == "" {
		return findings
	}
	findings = append(findings, c.leaderFinding(ctx, base))
	findings = append(findings, c.peersFinding(ctx, base)...)
	findings = append(findings, c.healthFindings(ctx, base)...)
	findings = append(findings, c.kvFindings(ctx, base)...)
	return findings
}

func (c *Check) leaderFinding(ctx context.Context, base string) engine.Finding {
	f := engine.Finding{Check: c.Name(), Target: "raft-leader"}
	_, body, err := c.get(ctx, base, "/v1/status/leader")
	if err != nil {
		return engine.Finding{Check: c.Name(), Target: "raft-leader", Status: engine.ERROR, Message: err.Error()}
	}
	var leader string
	_ = json.Unmarshal(body, &leader)
	if strings.TrimSpace(leader) == "" {
		f.Status, f.Message = engine.BAD, "no raft leader elected"
		return f
	}
	f.Status, f.Message = engine.OK, "leader: "+leader
	return f
}

func (c *Check) peersFinding(ctx context.Context, base string) []engine.Finding {
	_, body, err := c.get(ctx, base, "/v1/status/peers")
	if err != nil {
		return []engine.Finding{{Check: c.Name(), Target: "raft-peers", Status: engine.ERROR, Message: err.Error()}}
	}
	var peers []string
	_ = json.Unmarshal(body, &peers)
	f := engine.Finding{Check: c.Name(), Target: "raft-peers"}
	n := len(peers)
	switch {
	case c.cfg.ExpectPeers > 0 && n < (c.cfg.ExpectPeers/2+1):
		f.Status = engine.BAD
		f.Message = fmt.Sprintf("%d raft peers: quorum lost (want %d)", n, c.cfg.ExpectPeers)
	case c.cfg.ExpectPeers > 0 && n < c.cfg.ExpectPeers:
		f.Status = engine.WARN
		f.Message = fmt.Sprintf("%d/%d raft peers (one or more missing)", n, c.cfg.ExpectPeers)
	default:
		f.Status = engine.OK
		f.Message = fmt.Sprintf("%d raft peers", n)
	}
	return []engine.Finding{f}
}

func (c *Check) healthFindings(ctx context.Context, base string) []engine.Finding {
	var findings []engine.Finding
	findings = append(findings, c.stateFindings(ctx, base, "critical", engine.BAD)...)
	findings = append(findings, c.stateFindings(ctx, base, "warning", engine.WARN)...)
	return findings
}

func (c *Check) stateFindings(ctx context.Context, base, state string, status engine.Status) []engine.Finding {
	_, body, err := c.get(ctx, base, "/v1/health/state/"+state)
	if err != nil {
		return []engine.Finding{{Check: c.Name(), Target: "health/" + state, Status: engine.ERROR, Message: err.Error()}}
	}
	var checks []healthCheck
	_ = json.Unmarshal(body, &checks)
	var findings []engine.Finding
	for _, hc := range checks {
		findings = append(findings, engine.Finding{
			Check: c.Name(), Target: checkLabel(hc), Status: status,
			Message: fmt.Sprintf("check %s in state %s", nonEmpty(hc.Name, hc.CheckID), state),
		})
	}
	return findings
}

func (c *Check) kvFindings(ctx context.Context, base string) []engine.Finding {
	var findings []engine.Finding
	for _, key := range c.cfg.KVKeys {
		code, _, err := c.get(ctx, base, "/v1/kv/"+key)
		f := engine.Finding{Check: c.Name(), Target: "kv/" + key}
		switch {
		case err != nil:
			f.Status, f.Message = engine.ERROR, err.Error()
		case code == http.StatusNotFound:
			f.Status, f.Message = engine.BAD, "KV key missing"
		default:
			f.Status, f.Message = engine.OK, "present"
		}
		findings = append(findings, f)
	}
	return findings
}

// get performs a GET against base+path and returns the status code and body.
// A non-2xx (other than 404, which the caller may treat specially) is an error.
func (c *Check) get(ctx context.Context, target, path string) (int, []byte, error) {
	url := c.scheme() + "://" + target + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, nil, err
	}
	if c.cfg.TokenEnv != "" {
		if tok := os.Getenv(c.cfg.TokenEnv); tok != "" {
			req.Header.Set("X-Consul-Token", tok)
		}
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return resp.StatusCode, body, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return resp.StatusCode, body, nil
}

func checkLabel(hc healthCheck) string {
	svc := hc.ServiceName
	if svc == "" {
		svc = nonEmpty(hc.CheckID, "check")
	}
	if hc.Node != "" {
		return svc + "@" + hc.Node
	}
	return svc
}

func nonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
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
