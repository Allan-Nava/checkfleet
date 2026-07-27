package scaffold

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Allan-Nava/checkfleet/internal/inventory"
)

// fromHost renders one module's targets from a list of inventory hosts.
//
// Only modules whose target can be **derived from a hostname alone** are here.
// A postgres target needs a DSN with a user and a password: inventing one would
// produce a config that looks ready and fails on the first run, which is worse
// than saying so. Ask for an underivable module and FromInventory says which
// ones work instead.
var fromHost = map[string]func(hosts []inventory.Host) string{
	"certs": func(hosts []inventory.Host) string {
		var b strings.Builder
		b.WriteString("  certs:\n    warn_days: 30\n    crit_days: 7\n    targets:\n")
		for _, h := range hosts {
			fmt.Fprintf(&b, "      - %s:443\n", h.Address)
		}
		return strings.TrimRight(b.String(), "\n")
	},
	"tls": func(hosts []inventory.Host) string {
		var b strings.Builder
		b.WriteString("  tls:\n    warn_days: 30\n    crit_days: 7\n    targets:\n")
		for _, h := range hosts {
			fmt.Fprintf(&b, "      - %s:443\n", h.Address)
		}
		return strings.TrimRight(b.String(), "\n")
	},
	"http": func(hosts []inventory.Host) string {
		var b strings.Builder
		b.WriteString("  http:\n    targets:\n")
		for _, h := range hosts {
			fmt.Fprintf(&b, "      - {url: https://%s/, expect_status: 200}\n", h.Address)
		}
		return strings.TrimRight(b.String(), "\n")
	},
	"tcp": func(hosts []inventory.Host) string {
		var b strings.Builder
		b.WriteString("  tcp:\n    targets:\n")
		for _, h := range hosts {
			// The port is a placeholder on purpose: the inventory doesn't know
			// which service you meant.
			fmt.Fprintf(&b, "      - {name: %s, address: %s:22}\n", h.Name, h.Address)
		}
		return strings.TrimRight(b.String(), "\n")
	},
	"dns": func(hosts []inventory.Host) string {
		var b strings.Builder
		b.WriteString("  dns:\n    min_ttl_seconds: 60\n    targets:\n")
		for _, h := range hosts {
			fmt.Fprintf(&b, "      - {name: %s, type: A}\n", h.Name)
		}
		return strings.TrimRight(b.String(), "\n")
	},
	"ntp": func(hosts []inventory.Host) string {
		var b strings.Builder
		b.WriteString("  ntp:\n    offset_warn_ms: 100\n    offset_crit_ms: 1000\n    targets:\n")
		for _, h := range hosts {
			fmt.Fprintf(&b, "      - %s\n", h.Address)
		}
		return strings.TrimRight(b.String(), "\n")
	},
	"redis": func(hosts []inventory.Host) string {
		var b strings.Builder
		b.WriteString("  redis:\n    mem_warn_pct: 80\n    targets:\n")
		for _, h := range hosts {
			fmt.Fprintf(&b, "      - %s\n", h.Address)
		}
		return strings.TrimRight(b.String(), "\n")
	},
	"memcached": func(hosts []inventory.Host) string {
		var b strings.Builder
		b.WriteString("  memcached:\n    targets:\n")
		for _, h := range hosts {
			fmt.Fprintf(&b, "      - %s:11211\n", h.Address)
		}
		return strings.TrimRight(b.String(), "\n")
	},
	"haproxy": func(hosts []inventory.Host) string {
		var b strings.Builder
		b.WriteString("  haproxy:\n    targets:\n")
		for _, h := range hosts {
			fmt.Fprintf(&b, "      - http://%s:8404/stats\n", h.Address)
		}
		return strings.TrimRight(b.String(), "\n")
	},
	"consul": func(hosts []inventory.Host) string {
		var b strings.Builder
		b.WriteString("  consul:\n    targets:\n")
		for _, h := range hosts {
			fmt.Fprintf(&b, "      - http://%s:8500\n", h.Address)
		}
		return strings.TrimRight(b.String(), "\n")
	},
	"nats": func(hosts []inventory.Host) string {
		var b strings.Builder
		b.WriteString("  nats:\n    port: 8222\n    lag_warn: 100\n    lag_crit: 1000\n    targets:\n")
		for _, h := range hosts {
			fmt.Fprintf(&b, "      - %s\n", h.Address)
		}
		return strings.TrimRight(b.String(), "\n")
	},
}

// InventoryModules returns the modules FromInventory can generate, sorted.
func InventoryModules() []string {
	names := make([]string, 0, len(fromHost))
	for n := range fromHost {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// FromInventory renders a config whose targets are the inventory's hosts.
//
// This closes the gap between "the playbooks know every host" and "monitoring
// knows some of them": the inventory is already the source of truth, so the
// config should come from it rather than from someone retyping hostnames.
//
// No secrets are ever emitted — only hostnames and well-known ports.
func FromInventory(hosts []inventory.Host, modules []string) (string, error) {
	if len(hosts) == 0 {
		return "", fmt.Errorf("no hosts to scaffold from")
	}
	if len(modules) == 0 {
		modules = []string{"certs", "http"}
	}

	seen := map[string]bool{}
	var ordered []string
	for _, m := range modules {
		m = strings.ToLower(strings.TrimSpace(m))
		if m == "" || seen[m] {
			continue
		}
		if _, ok := fromHost[m]; !ok {
			if _, scaffoldable := snippets[m]; scaffoldable {
				return "", fmt.Errorf("module %q cannot be derived from an inventory (its targets need more than a hostname — a DSN, credentials or a bucket); "+
					"inventory-capable modules: %s", m, strings.Join(InventoryModules(), ", "))
			}
			return "", fmt.Errorf("unknown module %q; inventory-capable modules: %s", m, strings.Join(InventoryModules(), ", "))
		}
		seen[m] = true
		ordered = append(ordered, m)
	}
	if len(ordered) == 0 {
		return "", fmt.Errorf("no module selected")
	}

	// Stable output: sort hosts by name so regenerating the file produces the
	// same bytes and a diff shows only real inventory changes.
	sorted := append([]inventory.Host(nil), hosts...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	groups := map[string]bool{}
	for _, h := range sorted {
		if h.Group != "" {
			groups[h.Group] = true
		}
	}
	groupList := make([]string, 0, len(groups))
	for g := range groups {
		groupList = append(groupList, g)
	}
	sort.Strings(groupList)

	var b strings.Builder
	b.WriteString("# checkfleet.yml — generated by `checkfleet init --from-inventory`.\n")
	fmt.Fprintf(&b, "# %d host(s)", len(sorted))
	if len(groupList) > 0 {
		fmt.Fprintf(&b, " from group(s): %s", strings.Join(groupList, ", "))
	}
	b.WriteString("\n")
	b.WriteString("# Addresses come from `ansible_host` where the inventory sets one, otherwise\n")
	b.WriteString("# from the host name. For http/certs you may want the public DNS name instead\n")
	b.WriteString("# (SNI and the certificate CN follow the name, not the IP).\n")
	b.WriteString("# Ports and paths are placeholders — the inventory knows the hosts, not which\n")
	b.WriteString("# service you meant. Check them, then run:\n")
	b.WriteString("#   checkfleet check all --config checkfleet.yml\n")
	b.WriteString("# Secrets come from the environment (never inline).\n\n")
	b.WriteString("timeout_seconds: 30\n\n")
	b.WriteString("checks:\n")
	for i, m := range ordered {
		b.WriteString(fromHost[m](sorted))
		b.WriteString("\n")
		if i < len(ordered)-1 {
			b.WriteString("\n")
		}
	}
	return b.String(), nil
}
