package doctor

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/coverage"
	"github.com/Allan-Nava/checkfleet/internal/engine"
)

func findingFor(fs []engine.Finding, substr string) (engine.Finding, bool) {
	for _, f := range fs {
		if strings.Contains(f.Target, substr) {
			return f, true
		}
	}
	return engine.Finding{}, false
}

func TestEnvFindings(t *testing.T) {
	t.Setenv("CF_DOCTOR_SET", "value")

	refs := []engine.VarRef{
		{Name: "CF_DOCTOR_SET", Kind: engine.VarEnv, Resolved: true},
		{Name: "CF_DOCTOR_MISSING", Kind: engine.VarEnv},
		{Name: "CF_DOCTOR_DEFAULTED", Kind: engine.VarEnv, HasDefault: true},
		{Name: "/no/such/secret", Kind: engine.VarFile},
	}
	got := Env(refs)
	if len(got) != 4 {
		t.Fatalf("got %d findings, want 4", len(got))
	}

	// An unset variable with no default is BAD, not WARN: the config already
	// expanded it to "" and the check will fail blaming the service.
	if f, ok := findingFor(got, "CF_DOCTOR_MISSING"); !ok || f.Status != engine.BAD {
		t.Errorf("unset var: %+v, want BAD", f)
	} else if !strings.Contains(f.Message, "CF_DOCTOR_MISSING") {
		t.Errorf("the message must name the exact variable: %q", f.Message)
	}
	// With a default it still works, so it's only worth a WARN.
	if f, ok := findingFor(got, "CF_DOCTOR_DEFAULTED"); !ok || f.Status != engine.WARN {
		t.Errorf("defaulted var: %+v, want WARN", f)
	}
	if f, ok := findingFor(got, "CF_DOCTOR_SET"); !ok || f.Status != engine.OK {
		t.Errorf("set var: %+v, want OK", f)
	}
	if f, ok := findingFor(got, "/no/such/secret"); !ok || f.Status != engine.BAD {
		t.Errorf("missing secret file: %+v, want BAD", f)
	}
}

func TestConfigFindings(t *testing.T) {
	good := &engine.Config{}
	good.Checks.Certs = &engine.CertsConfig{Targets: []string{"example.com"}, WarnDays: 30, CritDays: 7}
	if got := Config(good, "checkfleet.yml"); len(got) != 1 || got[0].Status != engine.OK {
		t.Errorf("valid config should give one OK, got %+v", got)
	}

	bad := &engine.Config{}
	bad.Checks.HTTP = &engine.HTTPConfig{} // no targets
	got := Config(bad, "checkfleet.yml")
	if len(got) == 0 || got[0].Status != engine.BAD {
		t.Errorf("invalid config should give BAD findings, got %+v", got)
	}
}

func TestTargetsFindings(t *testing.T) {
	targets := []coverage.Target{
		// Port is set the way coverage.Targets() sets it, from the address.
		{Module: "http", Name: "https://a.example.com/", Hosts: []string{"a.example.com"}},
		{Module: "http", Name: "https://a.example.com/", Hosts: []string{"a.example.com"}}, // dupe
		{Module: "tcp", Name: "host:99999", Hosts: []string{"host"}, Port: 99999},          // bad port
		{Module: "consul", Name: "some/kv/key"},                                            // no host
	}
	got := Targets(targets)

	if f, ok := findingFor(got, "http"); !ok || f.Status != engine.WARN || !strings.Contains(f.Message, "duplicate") {
		t.Errorf("duplicate target: %+v, want WARN", f)
	}
	if f, ok := findingFor(got, "99999"); !ok || f.Status != engine.BAD {
		t.Errorf("implausible port: %+v, want BAD", f)
	}
	if f, ok := findingFor(got, "some/kv/key"); !ok || f.Status != engine.WARN {
		t.Errorf("hostless target: %+v, want WARN (it's legitimate for kv keys)", f)
	}
}

func TestPortOf(t *testing.T) {
	tests := []struct {
		in   string
		port int
		ok   bool
	}{
		{"https://example.com/health", 443, true}, // well-known for the scheme
		{"http://example.com/", 80, true},
		{"http://example.com:8080/path", 8080, true},
		{"redis://cache1", 6379, true},
		{"host:5432", 5432, true},
		{"example.com", 0, false},
		{"weird://host", 0, false},
	}
	for _, tt := range tests {
		port, ok := portOf(tt.in)
		if port != tt.port || ok != tt.ok {
			t.Errorf("portOf(%q) = %d,%v want %d,%v", tt.in, port, ok, tt.port, tt.ok)
		}
	}
}

// A listener that is up, and a port that is closed: the two outcomes an
// operator needs told apart. No external network.
func TestProbeUpAndDown(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()

	upPort := ln.Addr().(*net.TCPAddr).Port
	targets := []coverage.Target{
		{Module: "tcp", Name: net.JoinHostPort("127.0.0.1", itoa(upPort)), Hosts: []string{"127.0.0.1"}},
		{Module: "tcp", Name: "127.0.0.1:1", Hosts: []string{"127.0.0.1"}}, // nothing listens on 1
	}

	got := Probe(context.Background(), targets, 2*time.Second, 4)
	if len(got) != 2 {
		t.Fatalf("got %d findings, want 2: %+v", len(got), got)
	}

	up, ok := findingFor(got, itoa(upPort))
	if !ok || up.Status != engine.OK {
		t.Errorf("listener up: %+v, want OK", up)
	}
	down, ok := findingFor(got, "127.0.0.1:1")
	if !ok || down.Status != engine.ERROR {
		t.Errorf("closed port: %+v, want ERROR (we could not measure, not 'unhealthy')", down)
	} else if !strings.Contains(down.Message, "refuses TCP") {
		t.Errorf("the message should say the port refused: %q", down.Message)
	}
}

// A name that cannot resolve is ERROR, and says so — not "connection refused".
func TestProbeUnresolvableHost(t *testing.T) {
	targets := []coverage.Target{{
		Module: "http",
		// .invalid is reserved by RFC 2606 and never resolves.
		Name:  "https://checkfleet-doctor-test.invalid/",
		Hosts: []string{"checkfleet-doctor-test.invalid"},
	}}
	got := Probe(context.Background(), targets, 3*time.Second, 2)
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1", len(got))
	}
	if got[0].Status != engine.ERROR || !strings.Contains(got[0].Message, "does not resolve") {
		t.Errorf("unresolvable host: %+v, want ERROR mentioning resolution", got[0])
	}
}

// 40 URLs on one host must not become 40 probes.
func TestProbeDeduplicatesHostPort(t *testing.T) {
	var targets []coverage.Target
	for i := 0; i < 40; i++ {
		targets = append(targets, coverage.Target{
			Module: "http", Name: "https://127.0.0.1:1/page" + itoa(i), Hosts: []string{"127.0.0.1"},
		})
	}
	got := Probe(context.Background(), targets, time.Second, 4)
	if len(got) != 1 {
		t.Errorf("got %d probes for one host:port, want 1", len(got))
	}
}

func TestProbeSkipsResolutionForIPLiterals(t *testing.T) {
	// An IP with no port: resolve-only path, and an IP needs no resolving, so
	// this must be OK rather than a DNS error.
	targets := []coverage.Target{{Module: "certs", Name: "192.0.2.1", Hosts: []string{"192.0.2.1"}}}
	got := Probe(context.Background(), targets, time.Second, 2)
	if len(got) != 1 || got[0].Status != engine.OK {
		t.Errorf("IP literal without a port: %+v, want OK", got)
	}
	if !strings.Contains(got[0].Message, "no usable port") {
		t.Errorf("the message should explain why nothing was dialled: %q", got[0].Message)
	}
}

func TestScanVarsFindsUnsetReferences(t *testing.T) {
	t.Setenv("CF_SCAN_SET", "yes")

	dir := t.TempDir()
	path := filepath.Join(dir, "checkfleet.yml")
	body := `checks:
  postgres:
    targets:
      - {name: pg, dsn: "postgres://u:${CF_SCAN_MISSING}@db1:5432/x"}
      - {name: pg2, dsn: "postgres://u:${CF_SCAN_SET}@db2:5432/x"}
      - {name: pg3, dsn: "postgres://u:${CF_SCAN_DEFAULTED:-fallback}@db3:5432/x"}
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	refs, err := engine.ScanVars(path)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]engine.VarRef{}
	for _, r := range refs {
		byName[r.Name] = r
	}
	if r := byName["CF_SCAN_MISSING"]; !r.Missing() {
		t.Errorf("CF_SCAN_MISSING should be reported missing: %+v", r)
	}
	if r := byName["CF_SCAN_SET"]; !r.Resolved {
		t.Errorf("CF_SCAN_SET should be resolved: %+v", r)
	}
	if r := byName["CF_SCAN_DEFAULTED"]; r.Missing() {
		t.Errorf("a var with a default is not missing: %+v", r)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}
