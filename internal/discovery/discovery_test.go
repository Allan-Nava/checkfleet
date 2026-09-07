package discovery

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

func writeInventory(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "hosts.ini")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestResolveReadsAnInventory(t *testing.T) {
	inv := writeInventory(t, "[web]\nweb1 ansible_host=10.0.0.1\nweb2 ansible_host=10.0.0.2\n")
	hosts, err := Resolve(context.Background(), engine.Discovery{AnsibleInventory: inv})
	if err != nil {
		t.Fatal(err)
	}
	if got := Addresses(hosts); len(got) != 2 || got[0] != "10.0.0.1" || got[1] != "10.0.0.2" {
		t.Fatalf("want the two inventory hosts, got %v", got)
	}
	if hosts[0].Source != "inventory" || hosts[0].Group != "web" {
		t.Errorf("the source and group should survive resolution, got %+v", hosts[0])
	}
}

// The catalog is the point of CF-179: the fleet is already listed there, so a
// service lookup must produce targets without a line of checkfleet.yml per node.
func TestResolveReadsAConsulCatalog(t *testing.T) {
	var gotPath, gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		gotToken = r.Header.Get("X-Consul-Token")
		_, _ = w.Write([]byte(`[
		  {"Node":{"Node":"n1","Address":"10.0.0.1"},"Service":{"Address":"","Port":4222}},
		  {"Node":{"Node":"n2","Address":"10.0.0.2"},"Service":{"Address":"172.17.0.5","Port":4222}}
		]`))
	}))
	defer srv.Close()

	t.Setenv("CF_TEST_CONSUL_TOKEN", "s3cr3t")
	hosts, err := Resolve(context.Background(), engine.Discovery{ConsulService: &engine.ConsulService{
		Address: strings.TrimPrefix(srv.URL, "http://"), Service: "nats", Tag: "prod",
		TokenEnv: "CF_TEST_CONSUL_TOKEN",
	}})
	if err != nil {
		t.Fatal(err)
	}

	// The service's own address wins over the node's when it advertises one.
	want := []string{"10.0.0.1:4222", "172.17.0.5:4222"}
	got := Addresses(hosts)
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("want %v, got %v", want, got)
	}
	if !strings.Contains(gotPath, "/v1/health/service/nats") {
		t.Errorf("want the health endpoint, got %q", gotPath)
	}
	if !strings.Contains(gotPath, "passing=true") {
		t.Errorf("only_healthy defaults to true, so the query must ask for passing instances: %q", gotPath)
	}
	if !strings.Contains(gotPath, "tag=prod") {
		t.Errorf("the tag must narrow the lookup, got %q", gotPath)
	}
	if gotToken != "s3cr3t" {
		t.Errorf("the ACL token must travel in the header, got %q", gotToken)
	}
}

// only_healthy: false is the opposite request — list everything, including the
// instances failing their own checks.
func TestConsulCanListUnhealthyInstances(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	no := false
	if _, err := Resolve(context.Background(), engine.Discovery{ConsulService: &engine.ConsulService{
		Address: strings.TrimPrefix(srv.URL, "http://"), Service: "nats", OnlyHealthy: &no,
	}}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(gotPath, "passing") {
		t.Errorf("only_healthy: false must not ask for passing instances, got %q", gotPath)
	}
}

// A module that appends its own port (certs on 443) needs the discovered port
// dropped, or every target comes out as host:8500:443.
func TestConsulCanDropThePort(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"Node":{"Node":"n1","Address":"10.0.0.1"},"Service":{"Port":4222}}]`))
	}))
	defer srv.Close()

	no := false
	hosts, err := Resolve(context.Background(), engine.Discovery{ConsulService: &engine.ConsulService{
		Address: strings.TrimPrefix(srv.URL, "http://"), Service: "nats", KeepPort: &no,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got := Addresses(hosts); len(got) != 1 || got[0] != "10.0.0.1" {
		t.Fatalf("want the bare host, got %v", got)
	}
}

func TestConsulErrorsAreReported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	_, err := Resolve(context.Background(), engine.Discovery{ConsulService: &engine.ConsulService{
		Address: strings.TrimPrefix(srv.URL, "http://"), Service: "nats",
	}})
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("a rejected catalog lookup must surface, got %v", err)
	}
}

// One broken source must not hide the hosts the others resolved: a silently
// empty target list reports a healthy fleet, which is the worst failure mode
// this tool has.
func TestABrokenSourceKeepsTheOthers(t *testing.T) {
	inv := writeInventory(t, "web1 ansible_host=10.0.0.1\n")
	hosts, err := Resolve(context.Background(), engine.Discovery{
		AnsibleInventory: inv,
		ConsulService:    &engine.ConsulService{Address: "127.0.0.1:1", Service: "nats"},
	})
	if err == nil {
		t.Fatal("the unreachable catalog must be reported")
	}
	if got := Addresses(hosts); len(got) != 1 || got[0] != "10.0.0.1" {
		t.Fatalf("the inventory host must survive the catalog failure, got %v", got)
	}
}

// Both errors, not just the first tried: a config with two broken sources must
// say so, or fixing one reveals the other one run later.
func TestEverySourceErrorIsReported(t *testing.T) {
	_, err := Resolve(context.Background(), engine.Discovery{
		AnsibleInventory: filepath.Join(t.TempDir(), "nope.ini"),
		ConsulService:    &engine.ConsulService{Address: "127.0.0.1:1", Service: "nats"},
	})
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "nope.ini") || !strings.Contains(err.Error(), "consul service nats") {
		t.Fatalf("both failures must be named, got %v", err)
	}
}

// The same box listed by two sources is one target, and the first source wins
// so a hand-written inventory keeps its name.
func TestResolveDeduplicatesByAddress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"Node":{"Node":"catalog-name","Address":"10.0.0.1"},"Service":{"Port":0}}]`))
	}))
	defer srv.Close()

	inv := writeInventory(t, "web1 ansible_host=10.0.0.1\n")
	hosts, err := Resolve(context.Background(), engine.Discovery{
		AnsibleInventory: inv,
		ConsulService:    &engine.ConsulService{Address: strings.TrimPrefix(srv.URL, "http://"), Service: "nats"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 {
		t.Fatalf("the same address must collapse to one target, got %+v", hosts)
	}
	if hosts[0].Name != "web1" {
		t.Errorf("the first source should win, got %q", hosts[0].Name)
	}
}

func TestSetReportsConfiguredSources(t *testing.T) {
	if (engine.Discovery{}).Set() {
		t.Error("an empty discovery is not configured")
	}
	for _, d := range []engine.Discovery{
		{AnsibleInventory: "hosts.ini"},
		{ConsulService: &engine.ConsulService{Service: "nats"}},
		{DNSSRV: []engine.SRVLookup{{Name: "_nats._tcp"}}},
	} {
		if !d.Set() {
			t.Errorf("%+v should count as configured", d)
		}
	}
}

// Label keeps the inventory path while that is the only source, so findings
// that predate CF-179 read exactly as they did.
func TestLabelNamesTheSource(t *testing.T) {
	if got := Label(engine.Discovery{AnsibleInventory: "hosts.ini"}); got != "hosts.ini" {
		t.Errorf("want the inventory path, got %q", got)
	}
	mixed := engine.Discovery{AnsibleInventory: "hosts.ini", DNSSRV: []engine.SRVLookup{{Name: "_x._tcp"}}}
	if got := Label(mixed); got != "discovery" {
		t.Errorf("with several sources no single path is the answer, got %q", got)
	}
}

func TestConsulRequiresAService(t *testing.T) {
	_, err := Resolve(context.Background(), engine.Discovery{ConsulService: &engine.ConsulService{Address: "127.0.0.1:8500"}})
	if err == nil || !strings.Contains(err.Error(), "service is required") {
		t.Fatalf("want a clear error for a catalog lookup with no service, got %v", err)
	}
}
