package engine

import (
	"fmt"
	"net/url"
	"strings"
)

// Validate checks a loaded config for problems without running any check. It
// returns a list of human-readable issues; empty means the config is usable.
// It runs on the defaulted config, so threshold checks compare effective values.
func Validate(cfg *Config) []string {
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }

	c := cfg.Checks
	configured := 0

	if x := c.Certs; x != nil {
		configured++
		if len(x.Targets) == 0 && x.AnsibleInventory == "" {
			add("certs: nessun target né ansible_inventory")
		}
		if x.WarnDays < x.CritDays {
			add("certs: warn_days (%d) dovrebbe essere >= crit_days (%d)", x.WarnDays, x.CritDays)
		}
	}
	if x := c.HTTP; x != nil {
		configured++
		if len(x.Targets) == 0 {
			add("http: nessun target")
		}
		for i, t := range x.Targets {
			if t.URL == "" {
				add("http: target #%d senza url", i+1)
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
			add("stream: nessun target")
		}
		for i, t := range x.Targets {
			if t.URL == "" {
				add("stream: target #%d senza url", i+1)
			} else if _, err := url.Parse(t.URL); err != nil {
				add("stream: url non valido %q: %v", t.URL, err)
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
			add("postgres: nessun target")
		}
		for i, t := range x.Targets {
			if t.DSN == "" {
				add("postgres: target #%d (%s) senza dsn", i+1, t.Name)
			}
		}
		if x.LagWarnBytes > x.LagCritBytes {
			add("postgres: lag_warn_bytes > lag_crit_bytes")
		}
		if x.WraparoundWarnAge > x.WraparoundCritAge {
			add("postgres: wraparound_warn_age > wraparound_crit_age")
		}
		if x.ConnWarnPct < 0 || x.ConnWarnPct > 100 {
			add("postgres: conn_warn_pct (%d) fuori da 0-100", x.ConnWarnPct)
		}
	}
	if x := c.DNS; x != nil {
		configured++
		if len(x.Targets) == 0 {
			add("dns: nessun target")
		}
		for i, t := range x.Targets {
			if strings.TrimSpace(t.Name) == "" {
				add("dns: target #%d senza name", i+1)
			}
		}
	}

	if configured == 0 {
		add("nessun modulo configurato sotto `checks`")
	}
	return problems
}

func requireTargets(add func(string, ...any), module string, nTargets int, inventory string) {
	if nTargets == 0 && inventory == "" {
		add("%s: nessun target né ansible_inventory", module)
	}
}
