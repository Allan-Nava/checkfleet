package engine

import (
	"strings"
	"testing"
)

func TestValidateEmptyConfig(t *testing.T) {
	p := Validate(&Config{})
	if len(p) != 1 || !strings.Contains(p[0], "no module") {
		t.Errorf("empty config: want 1 problem 'no module', got %v", p)
	}
}

func TestValidateGoodConfig(t *testing.T) {
	cfg := &Config{Checks: ChecksConfig{
		Certs: &CertsConfig{WarnDays: 30, CritDays: 7, Targets: []string{"x:443"}},
		HTTP:  &HTTPConfig{Targets: []HTTPTarget{{URL: "https://x/"}}},
	}}
	if p := Validate(cfg); len(p) != 0 {
		t.Errorf("valid config: want 0 problems, got %v", p)
	}
}

func TestValidateMissingTargetsAndUrls(t *testing.T) {
	cfg := &Config{Checks: ChecksConfig{
		NATS:     &NATSConfig{},                                            // no targets/inventory
		HTTP:     &HTTPConfig{Targets: []HTTPTarget{{URL: ""}}},            // target without url
		Postgres: &PostgresConfig{Targets: []PostgresTarget{{Name: "db"}}}, // no dsn
	}}
	p := Validate(cfg)
	joined := strings.Join(p, "\n")
	for _, want := range []string{"nats: no target", "http: target #1 has no url", "postgres: target #1 (db) has no dsn"} {
		if !strings.Contains(joined, want) {
			t.Errorf("want problem %q, got:\n%s", want, joined)
		}
	}
}

func TestValidateThresholdOrder(t *testing.T) {
	cfg := &Config{Checks: ChecksConfig{
		Certs: &CertsConfig{WarnDays: 5, CritDays: 10, Targets: []string{"x"}}, // warn < crit
		NATS:  &NATSConfig{Targets: []string{"n"}, LagWarn: 1000, LagCrit: 100},
	}}
	joined := strings.Join(Validate(cfg), "\n")
	if !strings.Contains(joined, "certs: warn_days") {
		t.Errorf("want certs threshold problem, got:\n%s", joined)
	}
	if !strings.Contains(joined, "nats: lag_warn") {
		t.Errorf("want nats threshold problem, got:\n%s", joined)
	}
}
