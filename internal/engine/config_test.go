package engine

import (
	"os"
	"path/filepath"
	"testing"
)

// writeConfig writes body to a temp file and returns its path.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "checkfleet.yml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadConfigDefaults(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, `
checks:
  certs:
    targets: [example.com]
  http:
    targets:
      - url: https://example.com/
  nats:
    targets: [10.0.0.1]
  haproxy:
    targets: [10.0.0.2]
  patroni:
    targets: [10.0.0.3]
  consul:
    targets: [10.0.0.4]
  postgres:
    targets:
      - {name: db1, dsn: "host=x"}
  redis:
    targets: [10.0.0.5]
  stream:
    targets:
      - url: https://cdn/live.m3u8
        live: true
      - url: https://cdn/vod.m3u8
`))
	if err != nil {
		t.Fatal(err)
	}

	if cfg.TimeoutSeconds != 30 {
		t.Errorf("timeout default: want 30, got %d", cfg.TimeoutSeconds)
	}
	if cfg.Retries != 0 {
		t.Errorf("retries default: want 0, got %d", cfg.Retries)
	}
	if c := cfg.Checks.Certs; c.WarnDays != 30 || c.CritDays != 7 || c.Port != 443 {
		t.Errorf("certs default: %+v", c)
	}
	if cfg.Checks.HTTP.Targets[0].ExpectStatus != 200 {
		t.Errorf("http expect_status default: want 200, got %d", cfg.Checks.HTTP.Targets[0].ExpectStatus)
	}
	if n := cfg.Checks.NATS; n.Port != 8222 || n.LagWarn != 100 || n.LagCrit != 1000 {
		t.Errorf("nats default: %+v", n)
	}
	if hp := cfg.Checks.HAProxy; hp.Port != 8404 || hp.Path != "/stats;csv" {
		t.Errorf("haproxy default: %+v", hp)
	}
	if p := cfg.Checks.Patroni; p.Port != 8008 || p.LagWarnBytes != 32<<20 || p.LagCritBytes != 128<<20 {
		t.Errorf("patroni default: %+v", p)
	}
	if cn := cfg.Checks.Consul; cn.Port != 8500 {
		t.Errorf("consul default: %+v", cn)
	}
	if pg := cfg.Checks.Postgres; pg.LagWarnBytes != 32<<20 || pg.ConnWarnPct != 80 ||
		pg.WraparoundWarnAge != 1_500_000_000 || pg.SlotCritBytes != 2<<30 {
		t.Errorf("postgres default: %+v", pg)
	}
	if r := cfg.Checks.Redis; r.Port != 6379 || r.MemWarnPct != 80 || r.LagWarnBytes != 16<<20 {
		t.Errorf("redis default: %+v", r)
	}
	// Live target gets freshness defaults; non-live target must NOT.
	if s := cfg.Checks.Stream.Targets[0]; s.MaxAgeWarnSeconds != 30 || s.MaxAgeCritSeconds != 60 {
		t.Errorf("stream live default: %+v", s)
	}
	if s := cfg.Checks.Stream.Targets[1]; s.MaxAgeWarnSeconds != 0 || s.MaxAgeCritSeconds != 0 {
		t.Errorf("stream non-live non deve avere soglie di default: %+v", s)
	}
}

func TestLoadConfigKeepsExplicitValues(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, `
timeout_seconds: 5
checks:
  nats:
    port: 9999
    lag_warn: 7
    lag_crit: 42
  haproxy:
    port: 1940
    path: /haproxy?stats;csv
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TimeoutSeconds != 5 {
		t.Errorf("timeout esplicito perso: %d", cfg.TimeoutSeconds)
	}
	// retries>0 senza backoff esplicito → default 500ms.
	if c2, _ := LoadConfig(writeConfig(t, "retries: 2\nchecks:\n  certs:\n    targets: [x]\n")); c2.RetryBackoffMS != 500 {
		t.Errorf("retry_backoff default with retries>0: want 500, got %d", c2.RetryBackoffMS)
	}
	if n := cfg.Checks.NATS; n.Port != 9999 || n.LagWarn != 7 || n.LagCrit != 42 {
		t.Errorf("nats espliciti persi: %+v", n)
	}
	if hp := cfg.Checks.HAProxy; hp.Port != 1940 || hp.Path != "/haproxy?stats;csv" {
		t.Errorf("haproxy espliciti persi: %+v", hp)
	}
}

func TestLoadConfigAbsentModulesAreNil(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, "checks:\n  certs:\n    targets: [x]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Checks.HTTP != nil || cfg.Checks.NATS != nil || cfg.Checks.HAProxy != nil || cfg.Checks.Stream != nil || cfg.Checks.Patroni != nil || cfg.Checks.Consul != nil || cfg.Checks.Postgres != nil || cfg.Checks.DNS != nil || cfg.Checks.Redis != nil || cfg.Checks.Keycloak != nil || cfg.Checks.TCP != nil || cfg.Checks.TLS != nil || cfg.Checks.NTP != nil || cfg.Checks.RabbitMQ != nil || cfg.Checks.GRPC != nil || cfg.Checks.LDAP != nil || cfg.Checks.Kafka != nil {
		t.Errorf("i moduli non configurati devono restare nil: %+v", cfg.Checks)
	}
}

func TestLoadConfigErrors(t *testing.T) {
	if _, err := LoadConfig(filepath.Join(t.TempDir(), "missing.yml")); err == nil {
		t.Error("missing config: want error")
	}
	if _, err := LoadConfig(writeConfig(t, "checks: [not a map]\n")); err == nil {
		t.Error("invalid YAML: want error")
	}
}

func TestConfigInterpolation(t *testing.T) {
	t.Setenv("CF_TEST_PORT", "8443")
	dir := t.TempDir()
	secret := filepath.Join(dir, "pw")
	if err := os.WriteFile(secret, []byte("s3cr3t\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	body := "timeout_seconds: ${CF_TEST_TIMEOUT:-30}\n" +
		"checks:\n  certs:\n    port: ${CF_TEST_PORT}\n    targets:\n      - \"${file:" + secret + "}\"\n"
	cfg, err := LoadConfig(writeConfig(t, body))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TimeoutSeconds != 30 {
		t.Errorf("default interpolation: want 30, got %d", cfg.TimeoutSeconds)
	}
	if cfg.Checks.Certs.Port != 8443 {
		t.Errorf("env interpolation: want 8443, got %d", cfg.Checks.Certs.Port)
	}
	if len(cfg.Checks.Certs.Targets) != 1 || cfg.Checks.Certs.Targets[0] != "s3cr3t" {
		t.Errorf("file interpolation: want [s3cr3t], got %v", cfg.Checks.Certs.Targets)
	}
}

func TestConfigInterpolationMissingFile(t *testing.T) {
	body := "checks:\n  certs:\n    targets: [\"${file:/no/such/secret}\"]\n"
	if _, err := LoadConfig(writeConfig(t, body)); err == nil {
		t.Error("missing secret file: want error")
	}
}
