// Package doctor answers "why isn't this working?" before you blame the fleet.
//
// It reports on the **environment** rather than the services: unset variables,
// unreadable secret files, targets that can't be parsed, hosts that don't
// resolve, ports that refuse a connection. Same vocabulary as a check run
// (engine.Finding with OK/WARN/BAD/ERROR) so the existing renderers work
// unchanged, but the subject is your setup.
//
// It never gates: a diagnostic exits 0 (see the CLI). The point is to be
// runnable when everything is broken, including when the config won't load.
package doctor

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/coverage"
	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// check names, so the output groups sensibly.
const (
	checkEnv     = "env"
	checkConfig  = "config"
	checkTarget  = "target"
	checkNetwork = "network"
)

// Env turns a variable scan into findings. An unset ${NAME} with no default is
// BAD: the config expanded it to "" and the check will fail against an empty
// value, blaming the service for a missing variable.
func Env(refs []engine.VarRef) []engine.Finding {
	var out []engine.Finding
	for _, r := range refs {
		switch {
		case r.Kind == engine.VarFile && !r.Resolved:
			out = append(out, engine.Finding{
				Check: checkEnv, Target: "${file:" + r.Name + "}", Status: engine.BAD,
				Message: "secret file cannot be read: " + r.Name,
			})
		case r.Missing():
			out = append(out, engine.Finding{
				Check: checkEnv, Target: "${" + r.Name + "}", Status: engine.BAD,
				Message: fmt.Sprintf("environment variable %s is not set — it expands to an empty value", r.Name),
			})
		case !r.Resolved && r.HasDefault:
			out = append(out, engine.Finding{
				Check: checkEnv, Target: "${" + r.Name + "}", Status: engine.WARN,
				Message: fmt.Sprintf("%s is not set; the config's default is used", r.Name),
			})
		default:
			out = append(out, engine.Finding{
				Check: checkEnv, Target: "${" + r.Name + "}", Status: engine.OK,
				Message: "set",
			})
		}
	}
	return out
}

// Config turns engine.Validate's problems into findings, plus an OK when there
// are none.
func Config(cfg *engine.Config, path string) []engine.Finding {
	problems := engine.Validate(cfg)
	if len(problems) == 0 {
		return []engine.Finding{{
			Check: checkConfig, Target: path, Status: engine.OK, Message: "valid",
		}}
	}
	out := make([]engine.Finding, 0, len(problems))
	for _, p := range problems {
		out = append(out, engine.Finding{
			Check: checkConfig, Target: path, Status: engine.BAD, Message: p,
		})
	}
	return out
}

// Targets inspects the configured targets without touching the network:
// unparseable addresses, implausible ports and duplicates.
func Targets(targets []coverage.Target) []engine.Finding {
	var out []engine.Finding

	// Duplicates: the same module checking the same target twice is wasted work
	// and doubles every alert for it.
	seen := map[string]int{}
	for _, t := range targets {
		seen[t.Module+"\x00"+t.Name]++
	}
	var dupes []string
	for key, n := range seen {
		if n > 1 {
			module, name, _ := strings.Cut(key, "\x00")
			dupes = append(dupes, fmt.Sprintf("%s: %s (×%d)", module, name, n))
		}
	}
	sort.Strings(dupes)
	for _, d := range dupes {
		module, rest, _ := strings.Cut(d, ": ")
		out = append(out, engine.Finding{
			Check: checkTarget, Target: module, Status: engine.WARN,
			Message: "duplicate target " + rest,
		})
	}

	for _, t := range targets {
		if len(t.Hosts) == 0 {
			// Not always a bug — a Consul KV key or an S3 object legitimately
			// has no host — so WARN, with the reason stated.
			out = append(out, engine.Finding{
				Check: checkTarget, Target: t.Module + " " + t.Name, Status: engine.WARN,
				Message: "no host could be derived from this target, so it can't be probed",
			})
			continue
		}
		if port := t.Port; port != 0 && !plausiblePort(port) {
			out = append(out, engine.Finding{
				Check: checkTarget, Target: t.Module + " " + t.Name, Status: engine.BAD,
				Message: fmt.Sprintf("implausible port %d", port),
			})
		}
	}
	return out
}

// Probe resolves each distinct host and, where a port is known, opens a TCP
// connection to it. Findings are ERROR, not BAD: "we could not reach it from
// here" is a measurement failure — the same distinction the check modules make.
//
// One finding per host:port pair, deduplicated, so a config with 40 URLs on one
// host produces one line.
func Probe(ctx context.Context, targets []coverage.Target, timeout time.Duration, limit int) []engine.Finding {
	type probe struct {
		host string
		port int // 0 = resolve only
	}
	seen := map[probe]bool{}
	var todo []probe
	for _, t := range targets {
		// The port from the address wins; the label is the fallback for targets
		// whose Name is itself a URL, and finally a scheme's well-known port so
		// an https:// target is still probed.
		port := t.Port
		if port == 0 {
			port, _ = portOf(t.Name)
		}
		if !plausiblePort(port) {
			port = 0
		}
		for _, h := range t.Hosts {
			p := probe{host: h, port: port}
			if seen[p] {
				continue
			}
			seen[p] = true
			todo = append(todo, p)
		}
	}
	sort.Slice(todo, func(i, j int) bool {
		if todo[i].host != todo[j].host {
			return todo[i].host < todo[j].host
		}
		return todo[i].port < todo[j].port
	})

	if limit <= 0 {
		limit = 16
	}
	sem := make(chan struct{}, limit)
	out := make([]engine.Finding, len(todo))
	var wg sync.WaitGroup
	for i, p := range todo {
		wg.Add(1)
		go func(i int, p probe) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			out[i] = probeOne(ctx, p.host, p.port, timeout)
		}(i, p)
	}
	wg.Wait()
	return out
}

func probeOne(ctx context.Context, host string, port int, timeout time.Duration) engine.Finding {
	target := host
	if port > 0 {
		target = net.JoinHostPort(host, strconv.Itoa(port))
	}

	start := time.Now()
	// An IP literal needs no resolution; asking anyway would report a DNS
	// failure for something that was never a name.
	if net.ParseIP(host) == nil {
		rctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		if _, err := net.DefaultResolver.LookupHost(rctx, host); err != nil {
			return engine.Finding{
				Check: checkNetwork, Target: target, Status: engine.ERROR,
				Message: "does not resolve: " + cleanNetErr(err),
			}
		}
	}
	if port == 0 {
		// "no usable port" covers both an address that names none and one whose
		// port is implausible (reported separately by Targets) — in either case
		// there is nothing safe to dial.
		msg := "resolves; no usable port in the target, so no connection was tried"
		if net.ParseIP(host) != nil {
			msg = "is an IP literal; no usable port in the target, so no connection was tried"
		}
		return engine.Finding{Check: checkNetwork, Target: target, Status: engine.OK, Message: msg}
	}

	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", target)
	if err != nil {
		return engine.Finding{
			Check: checkNetwork, Target: target, Status: engine.ERROR,
			Message: "resolves but refuses TCP: " + cleanNetErr(err),
		}
	}
	_ = conn.Close()
	return engine.Finding{
		Check: checkNetwork, Target: target, Status: engine.OK,
		Message: fmt.Sprintf("TCP connect in %s", time.Since(start).Round(time.Millisecond)),
		Value:   engine.Num(float64(time.Since(start).Milliseconds())), Unit: "ms",
	}
}

// cleanNetErr trims Go's net error prose to the part an operator acts on.
func cleanNetErr(err error) string {
	s := err.Error()
	// "dial tcp 10.0.0.1:5432: connect: connection refused" → "connection refused"
	if i := strings.LastIndex(s, ": "); i >= 0 && i+2 < len(s) {
		return s[i+2:]
	}
	return s
}

// portOf extracts a port from a target label (a URL or host:port).
func portOf(name string) (int, bool) {
	if i := strings.Index(name, "://"); i >= 0 {
		rest := name[i+3:]
		if j := strings.IndexAny(rest, "/?"); j >= 0 {
			rest = rest[:j]
		}
		if _, p, err := net.SplitHostPort(rest); err == nil {
			if n, err := strconv.Atoi(p); err == nil {
				return n, true
			}
		}
		// A scheme with no explicit port: use the well-known one so a probe is
		// still possible.
		scheme := strings.ToLower(name[:i])
		if p, ok := schemePorts[scheme]; ok {
			return p, true
		}
		return 0, false
	}
	if _, p, err := net.SplitHostPort(name); err == nil {
		if n, err := strconv.Atoi(p); err == nil {
			return n, true
		}
	}
	return 0, false
}

var schemePorts = map[string]int{
	"http": 80, "https": 443,
	"redis": 6379, "rediss": 6380,
	"mongodb": 27017, "postgres": 5432, "postgresql": 5432, "mysql": 3306,
	"ldap": 389, "ldaps": 636,
	"amqp": 5672, "amqps": 5671,
	"rtmp": 1935,
}

func plausiblePort(p int) bool { return p > 0 && p <= 65535 }
