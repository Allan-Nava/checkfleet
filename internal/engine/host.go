package engine

import (
	"net"
	"strings"
)

// HostOf and SubnetOf live in engine rather than in insight because two
// features need the same answer: the blast-radius grouping (CF-123) and the
// dependency suppression (CF-174). Two copies of "what host is this target on"
// would drift, and they would drift silently — a target spelled one way in the
// grouping and another in the suppression looks like the feature simply not
// working.

// HostOf extracts the host from a target. Targets are spelled differently by
// module — "db-01:5432", "https://a.example/health", "pg-integration" — so this
// handles the shapes the modules actually produce and gives up otherwise.
func HostOf(target string) string {
	t := target
	// A module may qualify a sub-finding as "target/aspect" (postgres does).
	if i := strings.IndexByte(t, '/'); i > 0 && !strings.Contains(t, "://") {
		t = t[:i]
	}
	if strings.Contains(t, "://") {
		// URL: take the authority, drop credentials, port and path.
		rest := t[strings.Index(t, "://")+3:]
		if i := strings.IndexAny(rest, "/?#"); i >= 0 {
			rest = rest[:i]
		}
		if i := strings.LastIndexByte(rest, '@'); i >= 0 {
			rest = rest[i+1:]
		}
		t = rest
	}
	if h, _, err := net.SplitHostPort(t); err == nil {
		return h
	}
	if t == "" || strings.ContainsAny(t, " \t") {
		return ""
	}
	return t
}

// SubnetOf returns the /24 of an IPv4 target, or "" when the host is not a
// literal IPv4 address. Names are not resolved: an insight must not do DNS.
func SubnetOf(target string) string {
	ip := net.ParseIP(HostOf(target))
	if ip == nil {
		return ""
	}
	v4 := ip.To4()
	if v4 == nil {
		return ""
	}
	return v4.Mask(net.CIDRMask(24, 32)).String() + "/24"
}
