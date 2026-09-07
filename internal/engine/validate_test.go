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

// TestValidatePerModuleProblems exercises the error branch of every module that
// has explicit validation rules, plus the newer modules (smtp/es/mongodb) and
// the "module without rules still counts" path.
func TestValidatePerModuleProblems(t *testing.T) {
	cfg := &Config{Checks: ChecksConfig{
		Certs:         &CertsConfig{},                                    // no target/inventory
		Stream:        &StreamConfig{Targets: []StreamTarget{{URL: ""}}}, // no url
		HAProxy:       &HAProxyConfig{},                                  // no target
		Patroni:       &PatroniConfig{},                                  // no target
		Consul:        &ConsulConfig{},                                   // no target
		DNS:           &DNSConfig{Targets: []DNSTarget{{Name: ""}}},      // no name
		SMTP:          &SMTPConfig{},                                     // no target
		Elasticsearch: &ElasticsearchConfig{},                            // no target
		MongoDB:       &MongoDBConfig{},                                  // no target
	}}
	joined := strings.Join(Validate(cfg), "\n")
	for _, want := range []string{
		"certs: no target",
		"stream: target #1 has no url",
		"haproxy: no target",
		"patroni: no target",
		"consul: no target",
		"dns: target #1 has no name",
		"smtp: no target",
		"elasticsearch: no target",
		"mongodb: no target",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing problem %q in:\n%s", want, joined)
		}
	}
}

func TestValidateThresholdOrderAllModules(t *testing.T) {
	cfg := &Config{Checks: ChecksConfig{
		SMTP:          &SMTPConfig{Targets: []SMTPTarget{{Address: "x:25"}}, WarnDays: 5, CritDays: 10},
		Elasticsearch: &ElasticsearchConfig{Targets: []ElasticsearchTarget{{URL: "http://x"}}, DiskWarnPct: 95, DiskCritPct: 90},
		MongoDB:       &MongoDBConfig{Targets: []MongoDBTarget{{URI: "mongodb://x"}}, LagWarnSeconds: 100, LagCritSeconds: 10},
		Patroni:       &PatroniConfig{Targets: []string{"p"}, LagWarnBytes: 200, LagCritBytes: 100},
	}}
	joined := strings.Join(Validate(cfg), "\n")
	for _, want := range []string{
		"smtp: warn_days",
		"elasticsearch: disk_warn_pct",
		"mongodb: lag_warn_seconds",
		"patroni: lag_warn_bytes",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing threshold problem %q in:\n%s", want, joined)
		}
	}
}

// A module without explicit rules (e.g. tcp) still counts as configured, so an
// otherwise-empty config with only that module is valid.
func TestValidateRuleLessModuleCounts(t *testing.T) {
	cfg := &Config{Checks: ChecksConfig{TCP: &TCPConfig{Targets: []TCPTarget{{Address: "x:22"}}}}}
	if p := Validate(cfg); len(p) != 0 {
		t.Errorf("tcp-only config should be valid, got %v", p)
	}
}

func TestValidatePostgresRanges(t *testing.T) {
	cfg := &Config{Checks: ChecksConfig{Postgres: &PostgresConfig{
		Targets:     []PostgresTarget{{Name: "db", DSN: "host=x"}},
		ConnWarnPct: 150, // out of range
	}}}
	if !strings.Contains(strings.Join(Validate(cfg), "\n"), "conn_warn_pct") {
		t.Error("want conn_warn_pct range problem")
	}
}

// CF-180: a typo in an extraction selector must be caught by `validate`, not by
// a flow that has already logged in against production before failing.
func TestValidateCatchesFlowMistakes(t *testing.T) {
	cfg := &Config{Checks: ChecksConfig{Flow: &FlowConfig{Flows: []Flow{
		{Name: "login", Steps: []FlowStep{
			{Name: "token", URL: "https://auth.example.com/token",
				Extract: map[string]string{
					"good":       "json:access_token",
					"no-colon":   "access_token",
					"unknown":    "jq:.access_token",
					"no-capture": `regex:token="[^"]+"`,
					"bad-regex":  "regex:([",
				}},
			{Name: "no url"},
		}},
		{Name: "empty"},
	}}}}

	got := strings.Join(Validate(cfg), "\n")
	for _, want := range []string{
		`extract "no-colon"`,
		`extract "unknown"`,
		`extract "no-capture"`,
		`extract "bad-regex"`,
		"step 2 has no url",
		"flow empty: no steps",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, `extract "good"`) {
		t.Errorf("a valid selector was reported:\n%s", got)
	}
}

func TestValidateAcceptsAWorkingFlow(t *testing.T) {
	cfg := &Config{Checks: ChecksConfig{Flow: &FlowConfig{Flows: []Flow{
		{Name: "login", Steps: []FlowStep{
			{Name: "token", URL: "https://auth.example.com/token",
				Extract: map[string]string{"token": "json:access_token"}},
			{Name: "use", URL: "https://api.example.com/me"},
		}},
	}}}}
	if got := Validate(cfg); len(got) != 0 {
		t.Fatalf("a valid flow should raise nothing, got %v", got)
	}
}

func TestValidateCatchesMediaMTXMistakes(t *testing.T) {
	cfg := &Config{Checks: ChecksConfig{MediaMTX: &MediaMTXConfig{Targets: []MediaMTXTarget{
		{Name: "ok", URL: "http://mtx-01:9997"},
		{Name: "no-url"},
		{Name: "not-a-url", URL: "mtx-01:9997"},
		// A URL that already carries a path produces a 404 that reads like an
		// unreachable server, which is a slow thing to debug.
		{Name: "has-path", URL: "http://mtx-01:9997/v3/paths/list"},
		{Name: "half-auth", URL: "http://mtx-02:9997", Username: "admin"},
	}}}}

	got := strings.Join(Validate(cfg), "\n")
	for _, want := range []string{
		"mediamtx no-url: no url",
		"mediamtx not-a-url: url must be scheme+host",
		"mediamtx has-path: url should be the API base",
		"mediamtx half-auth: username set with no password_env",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "mediamtx ok:") {
		t.Errorf("a valid target was reported:\n%s", got)
	}
}
