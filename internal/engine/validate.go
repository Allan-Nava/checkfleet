package engine

import (
	"fmt"
	"net/url"
	"reflect"
	"strings"
)

// Validate checks a loaded config for problems without running any check. It
// returns a list of human-readable issues; empty means the config is usable.
// It runs on the defaulted config, so threshold checks compare effective values.
func Validate(cfg *Config) []string {
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }

	c := cfg.Checks
	// configured is kept per rule-bearing module below for readability; the
	// "nothing configured" check uses anyModuleConfigured so a module without
	// explicit rules (e.g. tcp, tls) still counts as a configured module.
	configured := 0

	if x := c.Certs; x != nil {
		configured++
		if len(x.Targets) == 0 && x.AnsibleInventory == "" {
			add("certs: no target or ansible_inventory")
		}
		if x.WarnDays < x.CritDays {
			add("certs: warn_days (%d) should be >= crit_days (%d)", x.WarnDays, x.CritDays)
		}
	}
	if x := c.HTTP; x != nil {
		configured++
		if len(x.Targets) == 0 {
			add("http: no target")
		}
		for i, t := range x.Targets {
			if t.URL == "" {
				add("http: target #%d has no url", i+1)
			}
		}
	}
	if x := c.NATS; x != nil {
		configured++
		requireTargets(add, "nats", len(x.Targets), x.AnsibleInventory)
		if x.LagWarn > x.LagCrit {
			add("nats: lag_warn (%d) > lag_crit (%d)", x.LagWarn, x.LagCrit)
		}
	}
	if x := c.HAProxy; x != nil {
		configured++
		requireTargets(add, "haproxy", len(x.Targets), x.AnsibleInventory)
	}
	if x := c.Stream; x != nil {
		configured++
		if len(x.Targets) == 0 {
			add("stream: no target")
		}
		for i, t := range x.Targets {
			if t.URL == "" {
				add("stream: target #%d has no url", i+1)
			} else if _, err := url.Parse(t.URL); err != nil {
				add("stream: invalid url %q: %v", t.URL, err)
			}
		}
	}
	if x := c.Patroni; x != nil {
		configured++
		requireTargets(add, "patroni", len(x.Targets), x.AnsibleInventory)
		if x.LagWarnBytes > x.LagCritBytes {
			add("patroni: lag_warn_bytes (%d) > lag_crit_bytes (%d)", x.LagWarnBytes, x.LagCritBytes)
		}
	}
	if x := c.Consul; x != nil {
		configured++
		requireTargets(add, "consul", len(x.Targets), x.AnsibleInventory)
	}
	if x := c.Postgres; x != nil {
		configured++
		if len(x.Targets) == 0 {
			add("postgres: no target")
		}
		for i, t := range x.Targets {
			if t.DSN == "" {
				add("postgres: target #%d (%s) has no dsn", i+1, t.Name)
			}
		}
		if x.LagWarnBytes > x.LagCritBytes {
			add("postgres: lag_warn_bytes > lag_crit_bytes")
		}
		if x.WraparoundWarnAge > x.WraparoundCritAge {
			add("postgres: wraparound_warn_age > wraparound_crit_age")
		}
		if x.ConnWarnPct < 0 || x.ConnWarnPct > 100 {
			add("postgres: conn_warn_pct (%d) out of range 0-100", x.ConnWarnPct)
		}
	}
	if x := c.DNS; x != nil {
		configured++
		if len(x.Targets) == 0 {
			add("dns: no target")
		}
		for i, t := range x.Targets {
			if strings.TrimSpace(t.Name) == "" {
				add("dns: target #%d has no name", i+1)
			}
		}
	}

	if x := c.SMTP; x != nil {
		configured++
		if len(x.Targets) == 0 {
			add("smtp: no target")
		}
		for i, t := range x.Targets {
			if strings.TrimSpace(t.Address) == "" {
				add("smtp: target #%d has no address", i+1)
			}
		}
		if x.WarnDays < x.CritDays {
			add("smtp: warn_days (%d) should be >= crit_days (%d)", x.WarnDays, x.CritDays)
		}
	}

	if x := c.Elasticsearch; x != nil {
		configured++
		if len(x.Targets) == 0 {
			add("elasticsearch: no target")
		}
		for i, t := range x.Targets {
			if strings.TrimSpace(t.URL) == "" {
				add("elasticsearch: target #%d has no url", i+1)
			}
		}
		if x.DiskWarnPct > x.DiskCritPct {
			add("elasticsearch: disk_warn_pct (%d) should be <= disk_crit_pct (%d)", x.DiskWarnPct, x.DiskCritPct)
		}
	}

	if x := c.MongoDB; x != nil {
		configured++
		if len(x.Targets) == 0 {
			add("mongodb: no target")
		}
		for i, t := range x.Targets {
			if strings.TrimSpace(t.URI) == "" {
				add("mongodb: target #%d has no uri", i+1)
			}
		}
		if x.LagWarnSeconds > x.LagCritSeconds {
			add("mongodb: lag_warn_seconds (%d) > lag_crit_seconds (%d)", x.LagWarnSeconds, x.LagCritSeconds)
		}
	}

	if x := c.MySQL; x != nil {
		configured++
		if len(x.Targets) == 0 {
			add("mysql: no target")
		}
		for i, t := range x.Targets {
			if strings.TrimSpace(t.DSN) == "" {
				add("mysql: target #%d has no dsn", i+1)
			}
		}
		if x.LagWarnSeconds > x.LagCritSeconds {
			add("mysql: lag_warn_seconds (%d) > lag_crit_seconds (%d)", x.LagWarnSeconds, x.LagCritSeconds)
		}
	}

	if x := c.Etcd; x != nil {
		configured++
		if len(x.Targets) == 0 {
			add("etcd: no target")
		}
		for i, t := range x.Targets {
			if strings.TrimSpace(t.URL) == "" {
				add("etcd: target #%d has no url", i+1)
			}
		}
	}

	if x := c.ClickHouse; x != nil {
		configured++
		if len(x.Targets) == 0 {
			add("clickhouse: no target")
		}
		for i, t := range x.Targets {
			if strings.TrimSpace(t.URL) == "" {
				add("clickhouse: target #%d has no url", i+1)
			}
		}
		if x.DelayWarnSeconds > x.DelayCritSeconds {
			add("clickhouse: delay_warn_seconds (%d) > delay_crit_seconds (%d)", x.DelayWarnSeconds, x.DelayCritSeconds)
		}
	}

	if x := c.Vault; x != nil {
		configured++
		if len(x.Targets) == 0 {
			add("vault: no target")
		}
		for i, t := range x.Targets {
			if strings.TrimSpace(t.URL) == "" {
				add("vault: target #%d has no url", i+1)
			}
		}
	}

	if x := c.Memcached; x != nil {
		configured++
		if len(x.Targets) == 0 {
			add("memcached: no target")
		}
		if x.MemWarnPct < 0 || x.MemWarnPct > 100 {
			add("memcached: mem_warn_pct (%d) out of range 0-100", x.MemWarnPct)
		}
		if x.EvictionsWarn < 0 {
			add("memcached: evictions_warn (%d) must not be negative", x.EvictionsWarn)
		}
	}

	if x := c.Cassandra; x != nil {
		configured++
		if len(x.Targets) == 0 {
			add("cassandra: no target")
		}
		for i, t := range x.Targets {
			if strings.TrimSpace(t.Address) == "" {
				add("cassandra: target #%d has no address", i+1)
			}
		}
		if x.ExpectNodes < 0 {
			add("cassandra: expect_nodes (%d) must not be negative", x.ExpectNodes)
		}
		// An expectation no set of targets can ever satisfy is a config mistake.
		if x.ExpectNodes > len(x.Targets) {
			add("cassandra: expect_nodes (%d) exceeds the %d configured target(s)", x.ExpectNodes, len(x.Targets))
		}
	}

	if configured == 0 && !anyModuleConfigured(c) {
		add("no module configured under `checks`")
	}
	return problems
}

func requireTargets(add func(string, ...any), module string, nTargets int, inventory string) {
	if nTargets == 0 && inventory == "" {
		add("%s: no target or ansible_inventory", module)
	}
}

// anyModuleConfigured reports whether at least one check module is set. Every
// ChecksConfig field is a *Config pointer, so a non-nil pointer means the module
// is present — this covers modules that have no explicit validation rules above.
func anyModuleConfigured(c ChecksConfig) bool {
	v := reflect.ValueOf(c)
	for i := 0; i < v.NumField(); i++ {
		if f := v.Field(i); f.Kind() == reflect.Pointer && !f.IsNil() {
			return true
		}
	}
	return false
}
