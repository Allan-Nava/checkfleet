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
		t.Errorf("timeout default: atteso 30, avuto %d", cfg.TimeoutSeconds)
	}
	if c := cfg.Checks.Certs; c.WarnDays != 30 || c.CritDays != 7 || c.Port != 443 {
		t.Errorf("certs default: %+v", c)
	}
	if cfg.Checks.HTTP.Targets[0].ExpectStatus != 200 {
		t.Errorf("http expect_status default: atteso 200, avuto %d", cfg.Checks.HTTP.Targets[0].ExpectStatus)
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
	if cfg.Checks.HTTP != nil || cfg.Checks.NATS != nil || cfg.Checks.HAProxy != nil || cfg.Checks.Stream != nil || cfg.Checks.Patroni != nil {
		t.Errorf("i moduli non configurati devono restare nil: %+v", cfg.Checks)
	}
}

func TestLoadConfigErrors(t *testing.T) {
	if _, err := LoadConfig(filepath.Join(t.TempDir(), "missing.yml")); err == nil {
		t.Error("config mancante: atteso errore")
	}
	if _, err := LoadConfig(writeConfig(t, "checks: [not a map]\n")); err == nil {
		t.Error("YAML invalido: atteso errore")
	}
}
