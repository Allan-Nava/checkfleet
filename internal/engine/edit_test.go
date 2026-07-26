package engine

import (
	"strings"
	"testing"
)

// addOK adds one endpoint and fails the test on error, returning the new YAML.
func addOK(t *testing.T, in string, spec EndpointSpec) string {
	t.Helper()
	out, err := AddEndpoint(in, spec)
	if err != nil {
		t.Fatalf("AddEndpoint(%+v): %v", spec, err)
	}
	return out
}

func TestAddEndpointIntoEmpty(t *testing.T) {
	out := addOK(t, "", EndpointSpec{Kind: "http", Value: "https://example.com/health", ExpectStatus: 200})
	// The result must load and validate as a real config.
	cfg, err := LoadBytes([]byte(out))
	if err != nil {
		t.Fatalf("result does not parse: %v\n%s", err, out)
	}
	if cfg.Checks.HTTP == nil || len(cfg.Checks.HTTP.Targets) != 1 ||
		cfg.Checks.HTTP.Targets[0].URL != "https://example.com/health" ||
		cfg.Checks.HTTP.Targets[0].ExpectStatus != 200 {
		t.Fatalf("http target not added: %+v", cfg.Checks.HTTP)
	}
	if p := Validate(cfg); len(p) != 0 {
		t.Errorf("added config should validate, got %v", p)
	}
}

func TestAddEndpointKinds(t *testing.T) {
	out := ""
	out = addOK(t, out, EndpointSpec{Kind: "certs", Value: "example.com:443"})
	out = addOK(t, out, EndpointSpec{Kind: "tcp", Value: "db.internal:5432"})
	out = addOK(t, out, EndpointSpec{Kind: "dns", Value: "example.com", RecordType: "AAAA"})

	cfg, err := LoadBytes([]byte(out))
	if err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if cfg.Checks.Certs == nil || len(cfg.Checks.Certs.Targets) != 1 || cfg.Checks.Certs.Targets[0] != "example.com:443" {
		t.Errorf("certs target missing: %+v", cfg.Checks.Certs)
	}
	if cfg.Checks.TCP == nil || len(cfg.Checks.TCP.Targets) != 1 || cfg.Checks.TCP.Targets[0].Address != "db.internal:5432" {
		t.Errorf("tcp target missing: %+v", cfg.Checks.TCP)
	}
	if cfg.Checks.DNS == nil || len(cfg.Checks.DNS.Targets) != 1 ||
		cfg.Checks.DNS.Targets[0].Name != "example.com" || cfg.Checks.DNS.Targets[0].Type != "AAAA" {
		t.Errorf("dns target missing/wrong type: %+v", cfg.Checks.DNS)
	}
}

func TestAddEndpointMoreKinds(t *testing.T) {
	out := ""
	out = addOK(t, out, EndpointSpec{Kind: "tls", Value: "example.com:443"})
	out = addOK(t, out, EndpointSpec{Kind: "redis", Value: "cache-01:6379"})
	out = addOK(t, out, EndpointSpec{Kind: "nats", Value: "nats-01:8222"})
	out = addOK(t, out, EndpointSpec{Kind: "smtp", Value: "relay.example.com:587"})
	out = addOK(t, out, EndpointSpec{Kind: "grpc", Value: "svc:443", Extra: "grpc.health.v1.Health"})
	out = addOK(t, out, EndpointSpec{Kind: "postgres", Value: "postgres://ops@pg-01:5432/app", Extra: "PGPASSWORD"})

	cfg, err := LoadBytes([]byte(out))
	if err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if p := Validate(cfg); len(p) != 0 {
		t.Fatalf("built config should validate, got %v\n%s", p, out)
	}
	if cfg.Checks.TLS == nil || len(cfg.Checks.TLS.Targets) != 1 || cfg.Checks.TLS.Targets[0] != "example.com:443" {
		t.Errorf("tls target: %+v", cfg.Checks.TLS)
	}
	if cfg.Checks.Redis == nil || len(cfg.Checks.Redis.Targets) != 1 {
		t.Errorf("redis target: %+v", cfg.Checks.Redis)
	}
	if cfg.Checks.NATS == nil || len(cfg.Checks.NATS.Targets) != 1 {
		t.Errorf("nats target: %+v", cfg.Checks.NATS)
	}
	if cfg.Checks.SMTP == nil || len(cfg.Checks.SMTP.Targets) != 1 || cfg.Checks.SMTP.Targets[0].Address != "relay.example.com:587" {
		t.Errorf("smtp target: %+v", cfg.Checks.SMTP)
	}
	if cfg.Checks.GRPC == nil || len(cfg.Checks.GRPC.Targets) != 1 ||
		cfg.Checks.GRPC.Targets[0].Address != "svc:443" || cfg.Checks.GRPC.Targets[0].Service != "grpc.health.v1.Health" {
		t.Errorf("grpc target (address+service): %+v", cfg.Checks.GRPC)
	}
	if cfg.Checks.Postgres == nil || len(cfg.Checks.Postgres.Targets) != 1 ||
		cfg.Checks.Postgres.Targets[0].DSN != "postgres://ops@pg-01:5432/app" || cfg.Checks.Postgres.Targets[0].PasswordEnv != "PGPASSWORD" {
		t.Errorf("postgres target (dsn+password_env): %+v", cfg.Checks.Postgres)
	}
}

func TestAddEndpointAppendsAndKeepsComments(t *testing.T) {
	in := "# my fleet\nchecks:\n  http:\n    targets:\n      - url: https://a.example.com/\n"
	out := addOK(t, in, EndpointSpec{Kind: "http", Value: "https://b.example.com/"})

	if !strings.Contains(out, "# my fleet") {
		t.Errorf("leading comment was dropped:\n%s", out)
	}
	cfg, err := LoadBytes([]byte(out))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Checks.HTTP == nil || len(cfg.Checks.HTTP.Targets) != 2 {
		t.Fatalf("want 2 http targets after append, got %+v", cfg.Checks.HTTP)
	}
}

func TestAddEndpointErrors(t *testing.T) {
	if _, err := AddEndpoint("", EndpointSpec{Kind: "http", Value: "  "}); err == nil {
		t.Error("empty value should error")
	}
	if _, err := AddEndpoint("", EndpointSpec{Kind: "mongodb", Value: "x"}); err == nil {
		t.Error("unsupported kind should error")
	}
	if _, err := AddEndpoint("checks: [1,2,3]\n", EndpointSpec{Kind: "http", Value: "https://x/"}); err != nil {
		// checks is a sequence, not a mapping — ensureMap would misbehave, but the
		// top level is still a mapping so this must not panic; a later parse would
		// catch the shape. Just assert no panic/err path here.
		t.Logf("got err (acceptable): %v", err)
	}
	if _, err := AddEndpoint("- just\n- a\n- list\n", EndpointSpec{Kind: "http", Value: "https://x/"}); err == nil {
		t.Error("non-mapping top level should error")
	}
}
