package insight

import (
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

func bad(check, target string) engine.Finding {
	return engine.Finding{Check: check, Target: target, Status: engine.BAD, Message: "x"}
}
func ok(check, target string) engine.Finding {
	return engine.Finding{Check: check, Target: target, Status: engine.OK, Message: "x"}
}

func TestOneDeadHostIsOneCluster(t *testing.T) {
	cs := Correlate([]engine.Finding{
		bad("postgres", "db-01:5432"),
		bad("redis", "db-01:6379"),
		bad("tcp", "db-01:22"),
		ok("http", "https://web-01/"),
	})
	if len(cs) != 1 {
		t.Fatalf("got %d clusters, want 1: %+v", len(cs), cs)
	}
	if cs[0].Dimension != "host" || cs[0].Value != "db-01" {
		t.Errorf("cluster = %s/%s, want host/db-01", cs[0].Dimension, cs[0].Value)
	}
	if cs[0].Size() != 3 {
		t.Errorf("size = %d, want 3", cs[0].Size())
	}
}

func TestOKFindingsAreNeverClustered(t *testing.T) {
	cs := Correlate([]engine.Finding{
		ok("postgres", "db-01:5432"), ok("redis", "db-01:6379"), ok("tcp", "db-01:22"),
	})
	if len(cs) != 0 {
		t.Errorf("healthy findings must not form a cluster: %+v", cs)
	}
}

func TestBelowThresholdIsNotAPattern(t *testing.T) {
	// Two findings on one host is a coincidence you read straight off the table.
	cs := Correlate([]engine.Finding{
		bad("postgres", "db-01:5432"), bad("redis", "db-01:6379"),
		bad("http", "https://a.example/"),
	})
	for _, c := range cs {
		if c.Size() < MinClusterSize {
			t.Errorf("cluster below the minimum leaked out: %+v", c)
		}
	}
}

func TestModuleWideFailureClustersByModule(t *testing.T) {
	cs := Correlate([]engine.Finding{
		bad("certs", "a.example:443"),
		bad("certs", "b.example:443"),
		bad("certs", "c.example:443"),
	})
	if len(cs) != 1 || cs[0].Dimension != "module" || cs[0].Value != "certs" {
		t.Fatalf("want one module cluster for certs, got %+v", cs)
	}
}

// TestEachFindingLandsInOneCluster: reporting the same outage under host and
// again under module is the same wall of rows with extra steps.
func TestEachFindingLandsInOneCluster(t *testing.T) {
	cs := Correlate([]engine.Finding{
		bad("postgres", "db-01:5432"), bad("redis", "db-01:6379"), bad("tcp", "db-01:22"),
		bad("certs", "a.example:443"), bad("certs", "b.example:443"), bad("certs", "c.example:443"),
	})
	seen := map[string]int{}
	total := 0
	for _, c := range cs {
		for _, f := range c.Findings {
			seen[f.Check+f.Target]++
			total++
		}
	}
	if total != 6 {
		t.Errorf("clusters cover %d findings, want 6", total)
	}
	for k, n := range seen {
		if n != 1 {
			t.Errorf("finding %s appears in %d clusters, want 1", k, n)
		}
	}
}

func TestSubnetGroupsLiteralIPv4(t *testing.T) {
	cs := Correlate([]engine.Finding{
		bad("tcp", "10.20.30.11:22"),
		bad("http", "http://10.20.30.12/health"),
		bad("redis", "10.20.30.13:6379"),
	})
	if len(cs) != 1 || cs[0].Dimension != "subnet" || cs[0].Value != "10.20.30.0/24" {
		t.Fatalf("want one 10.20.30.0/24 cluster, got %+v", cs)
	}
}

// TestHostExtraction covers engine.HostOf through the clustering that uses it:
// the function moved to engine so the dependency suppression (CF-174) could
// share it, and the behaviour this package depends on is still worth pinning
// from here.
func TestHostExtraction(t *testing.T) {
	cases := map[string]string{
		"db-01:5432":                  "db-01",
		"https://a.example/health":    "a.example",
		"http://user:pw@a.example:80": "a.example",
		"pg-integration":              "pg-integration",
		"db-01:5432/connections":      "db-01",
		"10.20.30.11:22":              "10.20.30.11",
	}
	for target, want := range cases {
		if got := engine.HostOf(target); got != want {
			t.Errorf("hostOf(%q) = %q, want %q", target, got, want)
		}
	}
}

func TestBiggestClusterComesFirst(t *testing.T) {
	cs := Correlate([]engine.Finding{
		bad("certs", "a:443"), bad("certs", "b:443"), bad("certs", "c:443"), bad("certs", "d:443"),
		bad("postgres", "db-01:5432"), bad("redis", "db-01:6379"), bad("tcp", "db-01:22"),
	})
	if len(cs) != 2 {
		t.Fatalf("want 2 clusters, got %+v", cs)
	}
	if cs[0].Size() < cs[1].Size() {
		t.Errorf("clusters not ordered by size: %d then %d", cs[0].Size(), cs[1].Size())
	}
}

func TestCorrelateIsStable(t *testing.T) {
	in := []engine.Finding{
		bad("postgres", "db-01:5432"), bad("redis", "db-01:6379"), bad("tcp", "db-01:22"),
		bad("certs", "a:443"), bad("certs", "b:443"), bad("certs", "c:443"),
	}
	first := Correlate(in)
	for i := 0; i < 5; i++ {
		next := Correlate(in)
		for j := range first {
			if next[j].Value != first[j].Value {
				t.Fatal("cluster order is not stable across runs")
			}
		}
	}
}
