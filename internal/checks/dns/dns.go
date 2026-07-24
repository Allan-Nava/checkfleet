// Package dns implements a DNS resolution health check using a minimal in-tree
// DNS client (no third-party dependency). For each target it queries one or
// more resolvers and flags: records that don't resolve, drift from an expected
// value, TTL below a threshold, and answers that diverge across resolvers
// (including SOA serial mismatches — a propagation problem).
package dns

import (
	"context"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// queryFunc runs one DNS query. It is injectable so the finding logic can be
// tested without touching the network.
type queryFunc func(ctx context.Context, resolver, name string, qtype uint16) ([]record, error)

type Check struct {
	cfg             engine.DNSConfig
	query           queryFunc
	systemResolvers func() []string
}

func New(cfg engine.DNSConfig) *Check {
	return &Check{cfg: cfg, query: liveQuery, systemResolvers: resolvConf}
}

func (c *Check) Name() string { return "dns" }

func (c *Check) Run(ctx context.Context) []engine.Finding {
	resolvers := c.resolvers()
	var findings []engine.Finding
	for _, t := range c.cfg.Targets {
		findings = append(findings, c.probe(ctx, t, resolvers)...)
	}
	return findings
}

func (c *Check) resolvers() []string {
	src := c.cfg.Resolvers
	if len(src) == 0 {
		src = c.systemResolvers()
	}
	out := make([]string, 0, len(src))
	for _, r := range src {
		if !strings.Contains(r, ":") {
			r += ":53"
		}
		out = append(out, r)
	}
	return out
}

func (c *Check) probe(ctx context.Context, t engine.DNSTarget, resolvers []string) []engine.Finding {
	typ := strings.ToUpper(strings.TrimSpace(t.Type))
	if typ == "" {
		typ = "A"
	}
	label := t.Name + "/" + typ
	qtype, ok := typeNumber(typ)
	if !ok {
		return []engine.Finding{{Check: c.Name(), Target: label, Status: engine.ERROR, Message: "tipo record non supportato: " + typ}}
	}
	if len(resolvers) == 0 {
		return []engine.Finding{{Check: c.Name(), Target: label, Status: engine.ERROR, Message: "nessun resolver configurato o di sistema"}}
	}

	// Query every resolver; keep each one's value set (records of the wanted
	// type) or its error.
	values := map[string][]string{}
	var firstErr error
	okCount := 0
	var minTTL uint32
	haveTTL := false
	for _, r := range resolvers {
		recs, err := c.query(ctx, r, t.Name, qtype)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		okCount++
		values[r] = valuesOfType(recs, typ)
		for _, rec := range recs {
			if rec.Type == typ {
				if !haveTTL || rec.TTL < minTTL {
					minTTL, haveTTL = rec.TTL, true
				}
			}
		}
	}
	if okCount == 0 {
		return []engine.Finding{{Check: c.Name(), Target: label, Status: engine.ERROR, Message: fmt.Sprintf("nessun resolver ha risposto: %v", firstErr)}}
	}

	var findings []engine.Finding
	findings = append(findings, c.resolutionFinding(label, typ, t.Expect, values))
	if f, ok := c.consistencyFinding(label, values, len(resolvers), okCount); ok {
		findings = append(findings, f)
	}
	if f, ok := c.ttlFinding(label, minTTL, haveTTL); ok {
		findings = append(findings, f)
	}
	return findings
}

func (c *Check) resolutionFinding(label, typ string, expect []string, values map[string][]string) engine.Finding {
	f := engine.Finding{Check: c.Name(), Target: label}
	rep := anyValue(values)

	if len(expect) > 0 {
		want := sortedUnique(expect)
		for r, got := range values {
			if !equal(got, want) {
				f.Status = engine.BAD
				f.Message = fmt.Sprintf("drift: atteso [%s], da %s [%s]", strings.Join(want, " "), r, strings.Join(got, " "))
				return f
			}
		}
		f.Status, f.Message = engine.OK, "come atteso: "+strings.Join(want, " ")
		return f
	}
	if len(rep) == 0 {
		f.Status, f.Message = engine.BAD, "nessun record "+typ
		return f
	}
	f.Status, f.Message = engine.OK, typ+" = "+strings.Join(rep, " ")
	return f
}

func (c *Check) consistencyFinding(label string, values map[string][]string, total, okCount int) (engine.Finding, bool) {
	f := engine.Finding{Check: c.Name(), Target: label + " [consistency]", Status: engine.WARN}
	// Distinct answer sets across the resolvers that responded.
	distinct := map[string][]string{}
	for r, v := range values {
		distinct[strings.Join(v, " ")] = append(distinct[strings.Join(v, " ")], r)
	}
	if len(distinct) > 1 {
		var parts []string
		for val, rs := range distinct {
			parts = append(parts, fmt.Sprintf("%s→[%s]", strings.Join(rs, ","), val))
		}
		sort.Strings(parts)
		f.Message = "risposte divergenti tra resolver: " + strings.Join(parts, " ; ")
		return f, true
	}
	if okCount < total {
		f.Message = fmt.Sprintf("%d/%d resolver non hanno risposto", total-okCount, total)
		return f, true
	}
	return engine.Finding{}, false
}

func (c *Check) ttlFinding(label string, minTTL uint32, haveTTL bool) (engine.Finding, bool) {
	if c.cfg.MinTTLSeconds == 0 || !haveTTL {
		return engine.Finding{}, false
	}
	f := engine.Finding{Check: c.Name(), Target: label + " [ttl]"}
	if minTTL < c.cfg.MinTTLSeconds {
		f.Status = engine.WARN
		f.Message = fmt.Sprintf("TTL minimo %ds sotto la soglia (%ds)", minTTL, c.cfg.MinTTLSeconds)
	} else {
		f.Status = engine.OK
		f.Message = fmt.Sprintf("TTL minimo %ds", minTTL)
	}
	return f, true
}

// liveQuery sends a real UDP query and parses the reply.
func liveQuery(ctx context.Context, resolver, name string, qtype uint16) ([]record, error) {
	q, err := buildQuery(name, qtype, 0x1234)
	if err != nil {
		return nil, err
	}
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "udp", resolver)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	} else {
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	}
	if _, err := conn.Write(q); err != nil {
		return nil, err
	}
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}
	rcode, answers, err := parseMessage(buf[:n], qtype)
	if err != nil {
		return nil, err
	}
	if rcode != 0 {
		return nil, fmt.Errorf("rcode %d (%s)", rcode, rcodeName(rcode))
	}
	return answers, nil
}

func rcodeName(rcode int) string {
	switch rcode {
	case 1:
		return "FORMERR"
	case 2:
		return "SERVFAIL"
	case 3:
		return "NXDOMAIN"
	case 5:
		return "REFUSED"
	default:
		return "?"
	}
}

// resolvConf returns the nameservers from /etc/resolv.conf.
func resolvConf() []string {
	data, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return nil
	}
	var ns []string
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "nameserver" {
			ns = append(ns, fields[1])
		}
	}
	return ns
}

func valuesOfType(recs []record, typ string) []string {
	var vals []string
	for _, r := range recs {
		if r.Type == typ {
			vals = append(vals, r.Value)
		}
	}
	return sortedUnique(vals)
}

func anyValue(values map[string][]string) []string {
	// Deterministic: return the entry with the lexicographically smallest key.
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return nil
	}
	return values[keys[0]]
}

func sortedUnique(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
