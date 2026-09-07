package discovery

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// srvLookup resolves one SRV record name into its targets.
//
// This uses the standard library's resolver rather than the hand-rolled wire
// decoder in internal/checks/dns. That decoder is unexported, shaped around the
// dns module's own Check, and — the deciding fact — has no SRV support at all:
// "reuse" here would mean writing and owning a second SRV rdata parser with
// name decompression, to answer a question net.Resolver already answers
// correctly. Still zero-dep either way.
func srvLookup(ctx context.Context, s engine.SRVLookup) ([]Host, error) {
	if s.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	r := net.DefaultResolver
	if s.Resolver != "" {
		r = resolverAt(s.Resolver)
	}
	// Empty service and proto means "look up Name directly", which is what a
	// config holding a full _nats._tcp.service.consul asks for.
	_, recs, err := r.LookupSRV(ctx, "", "", s.Name)
	if err != nil {
		return nil, err
	}
	hosts := make([]Host, 0, len(recs))
	for _, rec := range recs {
		target := strings.TrimSuffix(rec.Target, ".")
		if target == "" {
			continue
		}
		addr := target
		if rec.Port > 0 && s.Port() {
			addr = net.JoinHostPort(target, strconv.Itoa(int(rec.Port)))
		}
		hosts = append(hosts, Host{Name: target, Address: addr, Source: "dns-srv"})
	}
	return hosts, nil
}

// resolverAt sends queries to a specific nameserver. Names that only a
// service-discovery resolver serves — Consul's DNS on :8600, a cluster's
// CoreDNS — are invisible to the system resolver, so this is not an edge case.
func resolverAt(addr string) *net.Resolver {
	if !strings.Contains(addr, ":") {
		addr = net.JoinHostPort(addr, "53")
	}
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: 5 * time.Second}
			return d.DialContext(ctx, network, addr)
		},
	}
}
