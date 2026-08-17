package registry

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

const sentinel = "S3NT1NEL-PASSWORD-XYZ"

// A config whose every credential field holds the sentinel, pointed at a port
// nothing listens on so every module fails the way it fails in production.
const leakCfg = `
timeout_seconds: 2
checks:
  postgres:
    targets: [{name: pg, dsn: "host=127.0.0.1 port=1 user=u dbname=d sslmode=disable", password_env: CF_LEAK}]
  mysql:
    targets: [{name: my, dsn: "u:S3NT1NEL-PASSWORD-XYZ@tcp(127.0.0.1:1)/"}]
  mongodb:
    targets: [{name: mo, uri: "mongodb://127.0.0.1:1", username: u, password_env: CF_LEAK}]
  redis:
    port: 1
    password_env: CF_LEAK
    targets: [127.0.0.1]
  clickhouse:
    targets: [{name: ch, url: "http://127.0.0.1:1", username: u, password_env: CF_LEAK}]
  rabbitmq:
    port: 1
    username: u
    password_env: CF_LEAK
    targets: ["127.0.0.1"]
  elasticsearch:
    targets: [{name: es, url: "http://127.0.0.1:1", username: u, password_env: CF_LEAK}]
  consul:
    port: 1
    token_env: CF_LEAK
    targets: [127.0.0.1]
  vault:
    targets: [{name: va, url: "http://127.0.0.1:1", token_env: CF_LEAK}]
  haproxy:
    port: 1
    auth_user: u
    auth_pass_env: CF_LEAK
    targets: [127.0.0.1]
  ldap:
    targets: [{name: ld, url: "ldap://127.0.0.1:1", bind_dn: "cn=x", password_env: CF_LEAK}]
  s3:
    targets: [{name: s3t, endpoint: "http://127.0.0.1:1", bucket: b, access_key_env: CF_LEAK, secret_key_env: CF_LEAK}]
  kafka:
    brokers: ["127.0.0.1:1"]
    sasl_user: u
    sasl_mechanism: plain
    sasl_password_env: CF_LEAK
`

// TestNoFindingLeaksACredential is the guarantee CF-184 turns on. "The checks
// never log credentials" was a hand-kept convention; the tests that enforced it
// covered moduledoc and scaffold, not the finding messages — and a DSN inside a
// connection error is the classic way a password reaches a log.
func TestNoFindingLeaksACredential(t *testing.T) {
	t.Setenv("CF_LEAK", sentinel)
	cfg, err := engine.LoadBytes([]byte(leakCfg))
	if err != nil {
		t.Fatal(err)
	}
	checks := Configured(cfg)
	if len(checks) < 10 {
		t.Fatalf("only %d modules configured; the fixture is broken, not the code", len(checks))
	}
	// Concurrently: every module is waiting on a connection to a dead port, so
	// running them in series turns a 2s timeout into half a minute.
	// Short on purpose: c.Run is called directly, so the config's
	// timeout_seconds (which engine.Run applies) never comes into play here.
	// Every target is a dead port, so a few seconds is all any module needs.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var mu sync.Mutex
	var findings []engine.Finding
	var wg sync.WaitGroup
	for _, c := range checks {
		wg.Add(1)
		go func(c engine.Check) {
			defer wg.Done()
			out := c.Run(ctx)
			mu.Lock()
			findings = append(findings, out...)
			mu.Unlock()
		}(c)
	}
	wg.Wait()

	if len(findings) == 0 {
		t.Fatal("no findings produced; the fixture is broken, not the code")
	}
	for _, f := range findings {
		assertNoLeak(t, f)
	}
}

// assertNoLeak is the assertion, split out so a test can prove it bites.
func assertNoLeak(t testingT, f engine.Finding) {
	t.Helper()
	for field, v := range map[string]string{
		"message": f.Message, "target": f.Target,
		"runbook": f.Runbook, "remediation": f.Remediation,
	} {
		if strings.Contains(v, sentinel) {
			t.Errorf("%s/%s leaks the credential in %s:\n  %s", f.Check, f.Target, field, v)
		}
	}
}

// testingT is the slice of *testing.T the assertion needs, so the guard below
// can hand it a recorder instead.
type testingT interface {
	Helper()
	Errorf(format string, args ...any)
}

// recorder counts failures without failing the test that owns it.
type recorder struct{ failures int }

func (r *recorder) Helper()               {}
func (r *recorder) Errorf(string, ...any) { r.failures++ }

// TestTheLeakGuardBites proves the assertion above can reject something. A
// guarantee whose check nobody has seen fail is a convention with extra steps —
// which is exactly what CF-184 exists to replace.
func TestTheLeakGuardBites(t *testing.T) {
	var r recorder
	assertNoLeak(&r, engine.Finding{
		Check: "postgres", Target: "db",
		Message: "connection failed: dial user=u password=" + sentinel + " host=db",
	})
	if r.failures == 0 {
		t.Error("a finding containing the credential was accepted")
	}

	var clean recorder
	assertNoLeak(&clean, engine.Finding{Check: "postgres", Target: "db", Message: "connection refused"})
	if clean.failures != 0 {
		t.Error("a clean finding was rejected")
	}
}

// TestDSNModulesRedactTheirConnectionErrors is the narrower guard behind the
// sweep above. Those three modules carry the credential *inside* the connection
// string, so they are the ones where a driver error echoing its input turns
// into a leak. The sweep proves nothing leaks today; this proves the defence is
// wired in, so a future driver that starts echoing the DSN is already covered.
func TestDSNModulesRedactTheirConnectionErrors(t *testing.T) {
	for _, path := range []string{
		"../checks/mysql/mysql.go",
		"../checks/mongodb/mongodb.go",
		"../checks/postgres/postgres.go",
	} {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		body := string(src)
		i := strings.Index(body, `"connection failed: %v"`)
		if i < 0 {
			t.Errorf("%s: no connection-failed message found; did it move?", path)
			continue
		}
		// The redaction must be on that call, not merely somewhere in the file.
		line := body[i:min(i+120, len(body))]
		if !strings.Contains(line, "engine.Redact(") {
			t.Errorf("%s: the connection error is not redacted:\n  %s", path, line)
		}
	}
}
