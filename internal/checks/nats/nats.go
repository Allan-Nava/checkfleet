// Package nats implements a preflight/health check for a NATS JetStream
// cluster, reading the HTTP monitoring endpoints (/varz and /jsz?meta=1) of
// each node. It encodes the domain knowledge from the devops_hiway runbook:
// meta-leader present (and in the expected position), degraded/ghost peers
// (offline or not current), raft peer lag over threshold, and mixed binary
// versions across the cluster.
//
// Only the read-only monitoring port is used — the check never mutates the
// cluster.
package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/inventory"
)

type Check struct {
	cfg    engine.NATSConfig
	client *http.Client
}

func New(cfg engine.NATSConfig) *Check {
	return &Check{cfg: cfg, client: &http.Client{}}
}

func (c *Check) Name() string { return "nats" }

// varz is the subset of /varz we consume.
type varz struct {
	ServerName  string `json:"server_name"`
	Version     string `json:"version"`
	Host        string `json:"host"`
	Connections int    `json:"connections"`
	Uptime      string `json:"uptime"`
}

// jsz is the subset of /jsz?meta=1 we consume.
type jsz struct {
	MetaCluster metaCluster `json:"meta_cluster"`
}

type metaCluster struct {
	Name        string     `json:"name"`
	Leader      string     `json:"leader"`
	Replicas    []peerInfo `json:"replicas"`
	ClusterSize int        `json:"cluster_size"`
}

// peerInfo is one peer as seen from the responding node's perspective. In
// /jsz?meta=1 the responder lists every OTHER peer in `replicas`.
type peerInfo struct {
	Name    string `json:"name"`
	Current bool   `json:"current"`
	Offline bool   `json:"offline"`
	Active  int64  `json:"active"`
	Lag     uint64 `json:"lag"`
}

// nodeResult is what we fetched from a single monitoring endpoint.
type nodeResult struct {
	target string
	varz   *varz
	jsz    *jsz
	err    error
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
		findings = append(findings, engine.Finding{
			Check: c.Name(), Target: c.cfg.AnsibleInventory, Status: engine.ERROR, Message: err.Error(),
		})
	}
	results := c.fetchAll(ctx, targets)

	// 1) reachability + version per node; collect versions and meta views.
	versions := map[string][]string{}
	var views []metaView
	for _, r := range results {
		if r.err != nil {
			findings = append(findings, engine.Finding{
				Check: c.Name(), Target: r.target, Status: engine.ERROR,
				Message: fmt.Sprintf("monitoring not reachable: %v", r.err),
			})
			continue
		}
		findings = append(findings, engine.Finding{
			Check: c.Name(), Target: r.target, Status: engine.OK,
			Message: fmt.Sprintf("%s v%s, %d conn, up %s",
				r.varz.ServerName, r.varz.Version, r.varz.Connections, r.varz.Uptime),
		})
		if r.varz.Version != "" {
			label := r.varz.ServerName
			if label == "" {
				label = r.target
			}
			versions[r.varz.Version] = append(versions[r.varz.Version], label)
		}
		if r.jsz != nil {
			views = append(views, metaView{responder: r.varz.ServerName, mc: r.jsz.MetaCluster})
		}
	}

	// 2) mixed versions across the cluster.
	if len(versions) > 1 {
		findings = append(findings, engine.Finding{
			Check: c.Name(), Target: "cluster", Status: engine.WARN,
			Message: "mixed versions in the cluster: " + describeVersions(versions),
		})
	}

	// 3) meta-cluster: leader, expected position, peers, lag, ghosts.
	findings = append(findings, c.analyzeMeta(views)...)
	return findings
}

func (c *Check) fetchAll(ctx context.Context, targets []string) []nodeResult {
	results := make([]nodeResult, len(targets))
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

func (c *Check) fetch(ctx context.Context, target string) nodeResult {
	base := c.scheme() + "://" + target
	var v varz
	if err := c.getJSON(ctx, base+"/varz", &v); err != nil {
		return nodeResult{target: target, err: err}
	}
	// /jsz is best-effort: a reachable node without JetStream still counts as up.
	var j jsz
	if err := c.getJSON(ctx, base+"/jsz?meta=1", &j); err == nil {
		return nodeResult{target: target, varz: &v, jsz: &j}
	}
	return nodeResult{target: target, varz: &v}
}

func (c *Check) getJSON(ctx context.Context, url string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	return json.Unmarshal(body, dst)
}

func (c *Check) scheme() string {
	if c.cfg.Scheme != "" {
		return c.cfg.Scheme
	}
	return "http"
}

// metaView is one node's view of the meta-cluster.
type metaView struct {
	responder string
	mc        metaCluster
}

func (c *Check) analyzeMeta(views []metaView) []engine.Finding {
	if len(views) == 0 {
		return nil
	}
	var findings []engine.Finding

	// Leader: the set of non-empty leaders reported across views.
	leaders := map[string]bool{}
	for _, v := range views {
		if v.mc.Leader != "" {
			leaders[v.mc.Leader] = true
		}
	}
	switch {
	case len(leaders) == 0:
		findings = append(findings, engine.Finding{
			Check: c.Name(), Target: "meta-cluster", Status: engine.BAD,
			Message: "no meta-leader elected (quorum lost?)",
		})
	case len(leaders) > 1:
		findings = append(findings, engine.Finding{
			Check: c.Name(), Target: "meta-cluster", Status: engine.WARN,
			Message: "inconsistent meta-leader across nodes: " + joinSorted(leaders),
		})
	default:
		leader := joinSorted(leaders)
		status, msg := engine.OK, "meta-leader: "+leader
		if c.cfg.ExpectMetaLeader != "" && leader != c.cfg.ExpectMetaLeader {
			status = engine.WARN
			msg = fmt.Sprintf("meta-leader %s, want %s", leader, c.cfg.ExpectMetaLeader)
		}
		findings = append(findings, engine.Finding{
			Check: c.Name(), Target: "meta-cluster", Status: status, Message: msg,
		})
	}

	// Peers: union across views, worst observed status per peer.
	findings = append(findings, c.analyzePeers(views)...)
	return findings
}

func (c *Check) analyzePeers(views []metaView) []engine.Finding {
	worst := map[string]peerInfo{}
	members := map[string]bool{}
	for _, v := range views {
		if v.responder != "" {
			members[v.responder] = true
		}
		if v.mc.Leader != "" {
			members[v.mc.Leader] = true
		}
		for _, p := range v.mc.Replicas {
			members[p.Name] = true
			cur, ok := worst[p.Name]
			if !ok {
				worst[p.Name] = p
				continue
			}
			// Keep the least healthy observation and the highest lag.
			merged := cur
			merged.Offline = cur.Offline || p.Offline
			merged.Current = cur.Current && p.Current
			if p.Lag > merged.Lag {
				merged.Lag = p.Lag
			}
			worst[p.Name] = merged
		}
	}

	var findings []engine.Finding
	for _, name := range sortedKeys(worst) {
		findings = append(findings, c.peerFinding(name, worst[name]))
	}
	findings = append(findings, c.ghostFindings(members)...)
	return findings
}

func (c *Check) peerFinding(name string, p peerInfo) engine.Finding {
	f := engine.Finding{Check: c.Name(), Target: name}
	switch {
	case p.Offline:
		f.Status, f.Message = engine.BAD, "peer OFFLINE"
	case !p.Current:
		f.Status, f.Message = engine.WARN, fmt.Sprintf("peer not current (lag %d)", p.Lag)
	case c.cfg.LagCrit > 0 && p.Lag >= uint64(c.cfg.LagCrit):
		f.Status, f.Message = engine.BAD, fmt.Sprintf("lag %d over critical threshold (%d)", p.Lag, c.cfg.LagCrit)
	case c.cfg.LagWarn > 0 && p.Lag >= uint64(c.cfg.LagWarn):
		f.Status, f.Message = engine.WARN, fmt.Sprintf("lag %d over threshold (%d)", p.Lag, c.cfg.LagWarn)
	default:
		f.Status, f.Message = engine.OK, fmt.Sprintf("current, lag %d", p.Lag)
	}
	return f
}

// ghostFindings compares the observed members against expect_peers, if set:
// members not expected are ghosts, expected members absent are missing.
func (c *Check) ghostFindings(members map[string]bool) []engine.Finding {
	if len(c.cfg.ExpectPeers) == 0 {
		return nil
	}
	expected := map[string]bool{}
	for _, p := range c.cfg.ExpectPeers {
		expected[p] = true
	}
	var findings []engine.Finding
	for _, name := range sortedKeys(members) {
		if !expected[name] {
			findings = append(findings, engine.Finding{
				Check: c.Name(), Target: name, Status: engine.WARN,
				Message: "unexpected peer in the meta-cluster (ghost?)",
			})
		}
	}
	for _, name := range c.cfg.ExpectPeers {
		if !members[name] {
			findings = append(findings, engine.Finding{
				Check: c.Name(), Target: name, Status: engine.BAD,
				Message: "expected peer missing from the meta-cluster",
			})
		}
	}
	return findings
}

func withDefaultPort(target string, port int) string {
	if strings.Contains(target, ":") {
		return target
	}
	return fmt.Sprintf("%s:%d", target, port)
}

func describeVersions(versions map[string][]string) string {
	var parts []string
	for _, ver := range sortedKeys(versions) {
		parts = append(parts, fmt.Sprintf("v%s (%d)", ver, len(versions[ver])))
	}
	return strings.Join(parts, ", ")
}

func joinSorted(set map[string]bool) string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
