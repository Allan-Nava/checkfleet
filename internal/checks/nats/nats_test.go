package nats

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// fakeNode is one NATS monitoring endpoint (/varz + /jsz?meta=1).
type fakeNode struct {
	varz varz
	meta metaCluster
}

// startCluster spins an httptest server per node and returns their host:port
// targets. Each node reports its own /varz and the shared meta view.
func startCluster(t *testing.T, nodes ...fakeNode) []string {
	t.Helper()
	var targets []string
	for i := range nodes {
		node := nodes[i]
		mux := http.NewServeMux()
		mux.HandleFunc("/varz", func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(node.varz)
		})
		mux.HandleFunc("/jsz", func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(jsz{MetaCluster: node.meta})
		})
		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)
		targets = append(targets, strings.TrimPrefix(srv.URL, "http://"))
	}
	return targets
}

func run(t *testing.T, cfg engine.NATSConfig) []engine.Finding {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return New(cfg).Run(ctx)
}

// byTarget indexes findings by target for assertions.
func byTarget(findings []engine.Finding) map[string]engine.Finding {
	m := map[string]engine.Finding{}
	for _, f := range findings {
		m[f.Target] = f
	}
	return m
}

func healthyCluster() []fakeNode {
	replicas := []peerInfo{
		{Name: "gw-sg", Current: true},
		{Name: "gw-cl", Current: true},
		{Name: "gw-ov", Current: true},
	}
	nodes := make([]fakeNode, 3)
	for i, name := range []string{"gw-sg", "gw-cl", "gw-ov"} {
		var others []peerInfo
		for _, r := range replicas {
			if r.Name != name {
				others = append(others, r)
			}
		}
		nodes[i] = fakeNode{
			varz: varz{ServerName: name, Version: "2.14.3", Connections: 10, Uptime: "3d"},
			meta: metaCluster{Leader: "gw-cl", Replicas: others, ClusterSize: 3},
		}
	}
	return nodes
}

func TestHealthyCluster(t *testing.T) {
	targets := startCluster(t, healthyCluster()...)
	findings := run(t, engine.NATSConfig{Targets: targets, LagWarn: 100, LagCrit: 1000})

	if w := engine.Worst(findings); w != engine.OK {
		t.Fatalf("cluster sano: atteso worst OK, avuto %s\n%v", w, findings)
	}
	meta := byTarget(findings)["meta-cluster"]
	if meta.Status != engine.OK || !strings.Contains(meta.Message, "gw-cl") {
		t.Errorf("meta-cluster: atteso OK con leader gw-cl, avuto %s (%s)", meta.Status, meta.Message)
	}
}

func TestNoMetaLeaderIsBad(t *testing.T) {
	nodes := healthyCluster()
	for i := range nodes {
		nodes[i].meta.Leader = "" // quorum perso
	}
	findings := run(t, engine.NATSConfig{Targets: startCluster(t, nodes...), LagWarn: 100, LagCrit: 1000})
	meta := byTarget(findings)["meta-cluster"]
	if meta.Status != engine.BAD {
		t.Errorf("nessun leader: atteso BAD, avuto %s (%s)", meta.Status, meta.Message)
	}
}

func TestOfflinePeerIsBad(t *testing.T) {
	nodes := healthyCluster()
	// gw-cl (leader) reports gw-ov offline.
	for i := range nodes {
		if nodes[i].varz.ServerName != "gw-cl" {
			continue
		}
		for j := range nodes[i].meta.Replicas {
			if nodes[i].meta.Replicas[j].Name == "gw-ov" {
				nodes[i].meta.Replicas[j].Offline = true
				nodes[i].meta.Replicas[j].Current = false
			}
		}
	}
	findings := run(t, engine.NATSConfig{Targets: startCluster(t, nodes...), LagWarn: 100, LagCrit: 1000})
	if f := byTarget(findings)["gw-ov"]; f.Status != engine.BAD {
		t.Errorf("peer offline: atteso BAD, avuto %s (%s)", f.Status, f.Message)
	}
}

func TestPeerLagThresholds(t *testing.T) {
	nodes := healthyCluster()
	for i := range nodes {
		for j := range nodes[i].meta.Replicas {
			switch nodes[i].meta.Replicas[j].Name {
			case "gw-sg":
				nodes[i].meta.Replicas[j].Lag = 250 // > warn 100
			case "gw-ov":
				nodes[i].meta.Replicas[j].Lag = 5000 // > crit 1000
			}
		}
	}
	findings := byTarget(run(t, engine.NATSConfig{Targets: startCluster(t, nodes...), LagWarn: 100, LagCrit: 1000}))
	if f := findings["gw-sg"]; f.Status != engine.WARN {
		t.Errorf("lag 250: atteso WARN, avuto %s (%s)", f.Status, f.Message)
	}
	if f := findings["gw-ov"]; f.Status != engine.BAD {
		t.Errorf("lag 5000: atteso BAD, avuto %s (%s)", f.Status, f.Message)
	}
}

func TestMixedVersionsIsWarn(t *testing.T) {
	nodes := healthyCluster()
	nodes[0].varz.Version = "2.10.26" // il resto è 2.14.3
	findings := byTarget(run(t, engine.NATSConfig{Targets: startCluster(t, nodes...), LagWarn: 100, LagCrit: 1000}))
	f, ok := findings["cluster"]
	if !ok || f.Status != engine.WARN || !strings.Contains(f.Message, "versioni miste") {
		t.Errorf("versioni miste: atteso WARN, avuto %+v", f)
	}
}

func TestExpectedMetaLeaderMismatchIsWarn(t *testing.T) {
	findings := byTarget(run(t, engine.NATSConfig{
		Targets: startCluster(t, healthyCluster()...), LagWarn: 100, LagCrit: 1000,
		ExpectMetaLeader: "gw-sg", // reale è gw-cl
	}))
	if f := findings["meta-cluster"]; f.Status != engine.WARN || !strings.Contains(f.Message, "atteso gw-sg") {
		t.Errorf("leader inatteso: atteso WARN, avuto %s (%s)", f.Status, f.Message)
	}
}

func TestGhostAndMissingPeers(t *testing.T) {
	// Expect a peer set that includes an extra host and excludes gw-ov.
	findings := byTarget(run(t, engine.NATSConfig{
		Targets: startCluster(t, healthyCluster()...), LagWarn: 100, LagCrit: 1000,
		ExpectPeers: []string{"gw-sg", "gw-cl", "gw-extra"},
	}))
	if f := findings["gw-ov"]; f.Status != engine.WARN || !strings.Contains(f.Message, "ghost") {
		t.Errorf("gw-ov non atteso: atteso WARN ghost, avuto %s (%s)", f.Status, f.Message)
	}
	if f := findings["gw-extra"]; f.Status != engine.BAD || !strings.Contains(f.Message, "assente") {
		t.Errorf("gw-extra atteso ma assente: atteso BAD, avuto %s (%s)", f.Status, f.Message)
	}
}

func TestUnreachableNodeIsError(t *testing.T) {
	findings := run(t, engine.NATSConfig{Targets: []string{"127.0.0.1:1"}, LagWarn: 100, LagCrit: 1000})
	if len(findings) == 0 || findings[0].Status != engine.ERROR {
		t.Errorf("nodo irraggiungibile: atteso ERROR, avuto %v", findings)
	}
}

func TestTargetsDefaultPort(t *testing.T) {
	check := New(engine.NATSConfig{Port: 8222, Targets: []string{"a.example", "b.example:9000"}})
	targets, err := check.Targets()
	if err != nil {
		t.Fatal(err)
	}
	if targets[0] != "a.example:8222" || targets[1] != "b.example:9000" {
		t.Errorf("targets/porta di default sbagliati: %v", targets)
	}
}
