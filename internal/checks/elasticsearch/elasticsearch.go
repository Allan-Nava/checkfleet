// Package elasticsearch implements a health check for Elasticsearch/OpenSearch
// via their HTTP JSON API: cluster health colour, unassigned shards, expected
// node count and per-node disk watermark. Read-only, HTTP/JSON, zero third-party
// dependency.
package elasticsearch

import (
	"context"
	"crypto/tls"
	"encoding/json"
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
	cfg engine.ElasticsearchConfig
}

func New(cfg engine.ElasticsearchConfig) *Check { return &Check{cfg: cfg} }

func (c *Check) Name() string { return "elasticsearch" }

type clusterHealth struct {
	ClusterName                 string  `json:"cluster_name"`
	Status                      string  `json:"status"`
	NumberOfNodes               int     `json:"number_of_nodes"`
	UnassignedShards            int     `json:"unassigned_shards"`
	ActiveShardsPercentAsNumber float64 `json:"active_shards_percent_as_number"`
}

// allocRow is one row of _cat/allocation?format=json. _cat returns every field
// as a string, and the "UNASSIGNED" bucket has an empty disk.percent.
type allocRow struct {
	Node        string `json:"node"`
	DiskPercent string `json:"disk.percent"`
}

func (c *Check) Run(ctx context.Context) []engine.Finding {
	var findings []engine.Finding
	for _, t := range c.cfg.Targets {
		findings = append(findings, c.probe(ctx, t)...)
	}
	return findings
}

func (c *Check) probe(ctx context.Context, t engine.ElasticsearchTarget) []engine.Finding {
	label := t.Name
	if label == "" {
		label = hostOf(t.URL)
	}

	var h clusterHealth
	if err := c.get(ctx, t, "/_cluster/health", &h); err != nil {
		return []engine.Finding{{Check: c.Name(), Target: label, Status: engine.ERROR,
			Message: fmt.Sprintf("cluster health unreachable: %v", err)}}
	}

	var findings []engine.Finding
	f := engine.Finding{Check: c.Name(), Target: label}
	msg := fmt.Sprintf("%s: %d nodes, %d unassigned shards, %.0f%% active",
		h.Status, h.NumberOfNodes, h.UnassignedShards, h.ActiveShardsPercentAsNumber)
	switch h.Status {
	case "green":
		f.Status = engine.OK
	case "yellow":
		f.Status = engine.WARN
	case "red":
		f.Status = engine.BAD
	default:
		f.Status, msg = engine.ERROR, fmt.Sprintf("unknown cluster status %q", h.Status)
	}
	f.Message = msg
	findings = append(findings, f)

	// Expected node count (a shrunk cluster is a problem even if still green).
	if t.ExpectNodes > 0 && h.NumberOfNodes < t.ExpectNodes {
		findings = append(findings, engine.Finding{Check: c.Name(), Target: label + "/nodes",
			Status:  engine.BAD,
			Message: fmt.Sprintf("%d nodes present, expected %d", h.NumberOfNodes, t.ExpectNodes)})
	}

	findings = append(findings, c.diskFindings(ctx, t, label)...)
	return findings
}

func (c *Check) diskFindings(ctx context.Context, t engine.ElasticsearchTarget, label string) []engine.Finding {
	var rows []allocRow
	if err := c.get(ctx, t, "/_cat/allocation?format=json", &rows); err != nil {
		return []engine.Finding{{Check: c.Name(), Target: label + "/disk", Status: engine.ERROR,
			Message: fmt.Sprintf("allocation unreachable: %v", err)}}
	}
	var findings []engine.Finding
	for _, r := range rows {
		if r.DiskPercent == "" || r.Node == "" {
			continue // the UNASSIGNED bucket carries no disk figure
		}
		pct, err := strconv.Atoi(strings.TrimSpace(r.DiskPercent))
		if err != nil {
			continue
		}
		f := engine.Finding{Check: c.Name(), Target: label + "/disk/" + r.Node}
		switch {
		case pct >= c.cfg.DiskCritPct:
			f.Status, f.Message = engine.BAD, fmt.Sprintf("disk %d%% (over %d%% high watermark)", pct, c.cfg.DiskCritPct)
		case pct >= c.cfg.DiskWarnPct:
			f.Status, f.Message = engine.WARN, fmt.Sprintf("disk %d%% (over %d%% low watermark)", pct, c.cfg.DiskWarnPct)
		default:
			f.Status, f.Message = engine.OK, fmt.Sprintf("disk %d%%", pct)
		}
		findings = append(findings, f)
	}
	return findings
}

func (c *Check) get(ctx context.Context, t engine.ElasticsearchTarget, path string, dst any) error {
	base := strings.TrimRight(t.URL, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return err
	}
	if key := os.Getenv(t.APIKeyEnv); t.APIKeyEnv != "" && key != "" {
		req.Header.Set("Authorization", "ApiKey "+key)
	} else if t.Username != "" {
		req.SetBasicAuth(t.Username, os.Getenv(t.PasswordEnv))
	}

	resp, err := c.clientFor(t).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return err
	}
	return json.Unmarshal(body, dst)
}

func (c *Check) clientFor(t engine.ElasticsearchTarget) *http.Client {
	base := &tls.Config{MinVersion: tls.VersionTLS12}
	if t.Insecure {
		base.InsecureSkipVerify = true
	}
	if !c.cfg.ClientTLS.Set() && !t.Insecure {
		return &http.Client{}
	}
	// A bad certificate path surfaces as an ERROR finding on the first request
	// rather than here: this constructor has nowhere to report, and an ERROR is
	// the honest status for "the check could not measure".
	cfg, err := c.cfg.ClientTLS.Apply(base)
	if err != nil {
		return &http.Client{Transport: &errTransport{err: err}}
	}
	return &http.Client{Transport: &http.Transport{TLSClientConfig: cfg}}
}

// errTransport fails every request with a fixed error, so a misconfigured
// client certificate reads as an ERROR finding naming the problem.
type errTransport struct{ err error }

func (e *errTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, e.err }

func hostOf(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return u.Host
	}
	return raw
}
