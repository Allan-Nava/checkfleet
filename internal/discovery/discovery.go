// Package discovery resolves the set of hosts a module should check, from the
// places a fleet already keeps that list: an Ansible inventory, a Consul
// service catalog, DNS SRV records (CF-179).
//
// The point is to stop maintaining the fleet twice. A target list retyped into
// checkfleet.yml drifts the moment a node is added, and the drift is invisible:
// the check stays green because it never looked at the new host.
//
// Resolution happens once, before the run, so `checkfleet targets` can show
// exactly what a run would cover — the discovery answer is not a surprise
// buried in the results.
package discovery

import (
	"context"
	"fmt"
	"sort"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/inventory"
)

// Host is one discovered host and where it came from.
type Host struct {
	// Name identifies the host in output: the inventory hostname, the Consul
	// node name, the SRV target.
	Name string
	// Address is what to connect to.
	Address string
	// Source names the discovery source, for `checkfleet targets`: "inventory",
	// "consul", "dns-srv".
	Source string
	// Group is the inventory group, when the source has one.
	Group string
}

// Resolve gathers hosts from every configured source, deduplicated by address
// and sorted by address for a stable target order.
//
// Errors from different sources are joined rather than returned on the first
// one: a config with both an inventory and a catalog should say which of the
// two is broken, not just the first that happened to be tried.
func Resolve(ctx context.Context, d engine.Discovery) ([]Host, error) {
	var hosts []Host
	var errs []error

	if d.AnsibleInventory != "" {
		got, err := inventory.LoadPath(d.AnsibleInventory)
		if err != nil {
			errs = append(errs, fmt.Errorf("inventory %s: %w", d.AnsibleInventory, err))
		}
		for _, h := range got {
			hosts = append(hosts, Host{Name: h.Name, Address: h.Address, Source: "inventory", Group: h.Group})
		}
	}

	if d.ConsulService != nil {
		got, err := consulCatalog(ctx, *d.ConsulService)
		if err != nil {
			errs = append(errs, fmt.Errorf("consul service %s: %w", d.ConsulService.Service, err))
		}
		hosts = append(hosts, got...)
	}

	for _, srv := range d.DNSSRV {
		got, err := srvLookup(ctx, srv)
		if err != nil {
			errs = append(errs, fmt.Errorf("dns srv %s: %w", srv.Name, err))
		}
		hosts = append(hosts, got...)
	}

	return dedup(hosts), joinErrs(errs)
}

// dedup keeps the first host seen for each address. First wins so that an
// explicitly configured inventory beats whatever a catalog reports for the same
// box, and the Name stays the one a human wrote.
func dedup(in []Host) []Host {
	seen := map[string]bool{}
	out := make([]Host, 0, len(in))
	for _, h := range in {
		if h.Address == "" || seen[h.Address] {
			continue
		}
		seen[h.Address] = true
		out = append(out, h)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Address < out[j].Address })
	return out
}

func joinErrs(errs []error) error {
	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errs[0]
	}
	err := errs[0]
	for _, e := range errs[1:] {
		err = fmt.Errorf("%w; %w", err, e)
	}
	return err
}

// Addresses is the shortcut the check modules want: just the addresses.
func Addresses(hosts []Host) []string {
	out := make([]string, 0, len(hosts))
	for _, h := range hosts {
		out = append(out, h.Address)
	}
	return out
}

// Label names the discovery config in a finding's Target field when resolution
// fails. It stays the inventory path while that is the only source configured,
// so output and tests that predate the other sources read the same; with a
// catalog or SRV in play, naming one path would point at the wrong source.
func Label(d engine.Discovery) string {
	if d.AnsibleInventory != "" && d.ConsulService == nil && len(d.DNSSRV) == 0 {
		return d.AnsibleInventory
	}
	return "discovery"
}
