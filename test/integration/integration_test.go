//go:build integration

// Package integration exercises the check modules against the real services in
// docker-compose.integration.yml. It is OPT-IN: gated behind the `integration`
// build tag so `go test ./...` (unit tests, which must stay offline) never runs
// it. Bring the stack up first:
//
//	docker compose -f docker-compose.integration.yml up -d --wait
//	go test -tags integration ./test/integration/...
//
// The contract asserted per module is deliberately loose — connectivity, not
// exact status: a reachable service yields at least one non-ERROR finding
// (ERROR means "the check could not measure", i.e. the service was unreachable
// or unparseable). Exact status logic stays covered by the unit tests.
package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/checks/consul"
	"github.com/Allan-Nava/checkfleet/internal/checks/haproxy"
	"github.com/Allan-Nava/checkfleet/internal/checks/keycloak"
	"github.com/Allan-Nava/checkfleet/internal/checks/nats"
	"github.com/Allan-Nava/checkfleet/internal/checks/patroni"
	"github.com/Allan-Nava/checkfleet/internal/checks/postgres"
	"github.com/Allan-Nava/checkfleet/internal/checks/redis"
	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// configPath resolves the integration config, overridable via env.
func configPath(t *testing.T) string {
	if p := os.Getenv("CF_INTEGRATION_CONFIG"); p != "" {
		return p
	}
	p, err := filepath.Abs("../../checkfleet.integration.yml")
	if err != nil {
		t.Fatalf("resolve config path: %v", err)
	}
	return p
}

// loadConfig loads checkfleet.integration.yml once, ensuring the postgres
// password env the config references is set (defaults to the compose password).
func loadConfig(t *testing.T) *engine.Config {
	if os.Getenv("CF_PG_PASSWORD") == "" {
		t.Setenv("CF_PG_PASSWORD", "postgres")
	}
	cfg, err := engine.LoadConfig(configPath(t))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return cfg
}

// assertReachable runs the check and fails unless it produced at least one
// finding that is not ERROR (i.e. it actually measured the service).
func assertReachable(t *testing.T, c engine.Check) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	findings := c.Run(ctx)
	if len(findings) == 0 {
		t.Fatalf("%s: no findings (service not configured/reachable?)", c.Name())
	}
	measured := false
	for _, f := range findings {
		t.Logf("%s [%s] %s: %s", f.Check, f.Status, f.Target, f.Message)
		if f.Status != engine.ERROR {
			measured = true
		}
	}
	if !measured {
		t.Fatalf("%s: every finding is ERROR — service unreachable", c.Name())
	}
}

func TestRedis(t *testing.T) {
	cfg := loadConfig(t)
	if cfg.Checks.Redis == nil {
		t.Skip("redis not configured")
	}
	assertReachable(t, redis.New(*cfg.Checks.Redis))
}

func TestNATS(t *testing.T) {
	cfg := loadConfig(t)
	if cfg.Checks.NATS == nil {
		t.Skip("nats not configured")
	}
	assertReachable(t, nats.New(*cfg.Checks.NATS))
}

func TestConsul(t *testing.T) {
	cfg := loadConfig(t)
	if cfg.Checks.Consul == nil {
		t.Skip("consul not configured")
	}
	assertReachable(t, consul.New(*cfg.Checks.Consul))
}

func TestHAProxy(t *testing.T) {
	cfg := loadConfig(t)
	if cfg.Checks.HAProxy == nil {
		t.Skip("haproxy not configured")
	}
	assertReachable(t, haproxy.New(*cfg.Checks.HAProxy))
}

func TestPostgres(t *testing.T) {
	cfg := loadConfig(t)
	if cfg.Checks.Postgres == nil {
		t.Skip("postgres not configured")
	}
	assertReachable(t, postgres.New(*cfg.Checks.Postgres))
}

func TestPatroni(t *testing.T) {
	cfg := loadConfig(t)
	if cfg.Checks.Patroni == nil {
		t.Skip("patroni not configured")
	}
	assertReachable(t, patroni.New(*cfg.Checks.Patroni))
}

func TestKeycloak(t *testing.T) {
	cfg := loadConfig(t)
	if cfg.Checks.Keycloak == nil {
		t.Skip("keycloak not configured")
	}
	assertReachable(t, keycloak.New(*cfg.Checks.Keycloak))
}
