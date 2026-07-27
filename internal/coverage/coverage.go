// Package coverage flattens a config into the list of targets it actually
// checks, so an operator can answer "is everything I care about monitored?"
// without reading 600 lines of YAML.
//
// The enumeration is generic (reflection over ChecksConfig) rather than a
// hand-written case per module. A hand-written list is how you get a coverage
// tool that quietly under-reports: someone adds a module, forgets to add it
// here, and the tool answers "yes, all covered" because it never looked.
// TestEveryModuleYieldsTargets is the guard — it fails loudly when a new
// module's shape doesn't fit the convention below.
//
// The convention every module already follows:
//
//   - the targets live in a field named Targets (27 modules), Brokers (kafka)
//     or BaseURL (keycloak);
//   - that field is a []string, a []SomethingTarget, or a plain string;
//   - inside a target struct, the display name is Name (or URL when there is
//     no Name), and the address is one of Address, URL, DSN, URI, Endpoint.
package coverage

import (
	"net"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/inventory"
)

// Target is one configured check target, flattened across modules.
type Target struct {
	Module string `json:"module"`
	Name   string `json:"name"`
	// Hosts are the bare hostnames this target reaches, for diffing against an
	// inventory. Usually one, but a connection string can legitimately name
	// several — a MongoDB replica-set URI lists every member, and all of them
	// are covered by that single target. Empty when no host is derivable (an
	// object key, a queue name…).
	Hosts []string `json:"hosts,omitempty"`
	// Port from the target's address, 0 when it names none. Carried separately
	// because the address itself may be a DSN that must never be printed, while
	// the port is plain metadata a diagnostic needs in order to probe. When an
	// address lists several endpoints (a replica set) this is the first one's
	// port, which in practice is shared by all members.
	Port int `json:"port,omitempty"`
}

// candidate field names holding targets, in priority order.
var targetFields = []string{"Targets", "Brokers", "BaseURL"}

// address field names inside a target struct, in priority order.
var addressFields = []string{"Address", "URL", "Endpoint", "DSN", "URI"}

// Targets enumerates every target of every configured module, in registry
// order (the order of the ChecksConfig fields).
//
// Note what is *not* returned: the raw value of a DSN or URI field. Those carry
// credentials for postgres/mysql/mongodb, and this list is printed to a
// terminal, a JSON file and a CI log. Only the extracted hostname travels.
func Targets(cfg *engine.Config) []Target {
	if cfg == nil {
		return nil
	}
	var out []Target
	checks := reflect.ValueOf(&cfg.Checks).Elem()
	ct := checks.Type()
	for i := 0; i < checks.NumField(); i++ {
		f := checks.Field(i)
		if f.Kind() != reflect.Pointer || f.IsNil() {
			continue
		}
		module := yamlName(ct.Field(i))
		out = append(out, targetsOf(module, f.Elem())...)
	}
	return out
}

// yamlName is the module's config key, which is also its registry name.
func yamlName(f reflect.StructField) string {
	tag := f.Tag.Get("yaml")
	if name, _, _ := strings.Cut(tag, ","); name != "" {
		return name
	}
	return strings.ToLower(f.Name)
}

func targetsOf(module string, cfg reflect.Value) []Target {
	for _, name := range targetFields {
		f := cfg.FieldByName(name)
		if !f.IsValid() {
			continue
		}
		switch f.Kind() {
		case reflect.String:
			if s := f.String(); s != "" {
				return []Target{{Module: module, Name: s, Hosts: hostsOf(s), Port: portOf(s)}}
			}
		case reflect.Slice:
			out := make([]Target, 0, f.Len())
			for i := 0; i < f.Len(); i++ {
				if t, ok := targetFrom(module, f.Index(i)); ok {
					out = append(out, t)
				}
			}
			return out
		}
	}
	return nil
}

func targetFrom(module string, v reflect.Value) (Target, bool) {
	if v.Kind() == reflect.String {
		s := v.String()
		if s == "" {
			return Target{}, false
		}
		return Target{Module: module, Name: s, Hosts: hostsOf(s), Port: portOf(s)}, true
	}
	if v.Kind() != reflect.Struct {
		return Target{}, false
	}

	// The address first: it is also the fallback display name.
	addr := ""
	for _, name := range addressFields {
		if f := v.FieldByName(name); f.IsValid() && f.Kind() == reflect.String && f.String() != "" {
			addr = f.String()
			break
		}
	}
	name := ""
	if f := v.FieldByName("Name"); f.IsValid() && f.Kind() == reflect.String {
		name = f.String()
	}
	if name == "" {
		// No explicit name: show the address, but never a credential-bearing
		// one — those get the host only.
		name = safeName(addr)
	}
	if name == "" {
		return Target{}, false
	}
	if addr == "" {
		// A target struct with no address field at all: the name *is* the thing
		// being reached. dns is the case that exists today (DNSTarget is
		// {Name, Type, Expect} and Name is the domain to resolve), and without
		// this a dns target could never match an inventory host.
		addr = name
	}
	return Target{Module: module, Name: name, Hosts: hostsOf(addr), Port: portOf(addr)}, true
}

// safeName is the address as a display label, reduced to its host when it could
// embed credentials (anything with userinfo).
func safeName(addr string) string {
	if addr == "" {
		return ""
	}
	if u, err := url.Parse(addr); err == nil && u.User != nil {
		return u.Hostname()
	}
	return addr
}

// hostsOf extracts the bare hostnames from every shape a target address takes
// in this config format. Credentials never survive: the URL path drops userinfo
// via url.Hostname(), and the DSN paths read only the address portion.
//
// The shapes, all of which appear in checkfleet.example.yml:
//
//	https://api.example.com/health                  → api.example.com
//	mongodb://a:27017,b:27017,c:27017/?replicaSet=rs → a, b, c   (all covered)
//	monitor:${PASS}@tcp(db-01:3306)/                → db-01      (Go MySQL DSN)
//	host=10.20.30.11 port=5432 user=monitor         → 10.20.30.11 (libpq)
//	db1.internal:5432                               → db1.internal
func hostsOf(addr string) []string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return nil
	}

	// Go MySQL DSN: the address lives inside net(...), e.g. tcp(db-01:3306).
	// Checked before the URL branch because such a DSN has no "://" but does
	// have userinfo, which would otherwise confuse the bare-host path.
	if open := strings.IndexByte(addr, '('); open >= 0 {
		if close := strings.IndexByte(addr[open:], ')'); close > 0 {
			return hostsOf(addr[open+1 : open+close])
		}
	}

	if strings.Contains(addr, "://") {
		u, err := url.Parse(addr)
		if err != nil || u.Host == "" {
			return nil
		}
		// u.Host excludes userinfo but may be a comma-separated list of
		// host:port pairs (replica sets), which url.Hostname() mangles.
		var out []string
		for _, part := range strings.Split(u.Host, ",") {
			if h := bareHost(part); h != "" {
				out = append(out, h)
			}
		}
		return out
	}

	// A key=value DSN ("host=db1 user=…"), as libpq accepts.
	if strings.Contains(addr, "=") {
		for _, field := range strings.Fields(addr) {
			if k, v, ok := strings.Cut(field, "="); ok && k == "host" && v != "" {
				return []string{v}
			}
		}
		return nil
	}

	if h := bareHost(addr); h != "" {
		return []string{h}
	}
	return nil
}

// portOf extracts the port from an address, following the same shapes as
// hostsOf. 0 means the address names no port.
func portOf(addr string) int {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return 0
	}
	if open := strings.IndexByte(addr, '('); open >= 0 {
		if close := strings.IndexByte(addr[open:], ')'); close > 0 {
			return portOf(addr[open+1 : open+close])
		}
	}

	hostport := addr
	if strings.Contains(addr, "://") {
		u, err := url.Parse(addr)
		if err != nil || u.Host == "" {
			return 0
		}
		// The first endpoint of a possible comma-separated list.
		hostport, _, _ = strings.Cut(u.Host, ",")
	} else if strings.Contains(addr, "=") {
		for _, field := range strings.Fields(addr) {
			if k, v, ok := strings.Cut(field, "="); ok && k == "port" {
				if n, err := strconv.Atoi(v); err == nil {
					return n
				}
			}
		}
		return 0
	}
	if _, p, err := net.SplitHostPort(hostport); err == nil {
		if n, err := strconv.Atoi(p); err == nil {
			return n
		}
	}
	return 0
}

// bareHost reduces "host:port", "[::1]:port" or "host" to the host.
func bareHost(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(s); err == nil {
		return host
	}
	// No port, or an unparseable one. Refuse anything still carrying path,
	// query or userinfo syntax rather than guessing at it.
	if strings.ContainsAny(s, "/?@ ") {
		return ""
	}
	return strings.Trim(s, "[]")
}

// Diff compares the configured targets against an inventory's hosts.
type Diff struct {
	// Covered are inventory hosts that at least one target points at.
	Covered []string `json:"covered"`
	// Uncovered are inventory hosts no target mentions — the answer to "is
	// every prod host monitored?".
	Uncovered []string `json:"uncovered"`
	// Extra are hosts checkfleet targets that the inventory doesn't list:
	// usually external dependencies, sometimes a stale or misspelled target.
	Extra []string `json:"extra"`
}

// DiffInventory matches targets against inventory host names *and* addresses,
// because an inventory entry may be reached by either (`web1 ansible_host=10.0.0.5`
// is one host, and a target may name it as `web1` or as `10.0.0.5`).
func DiffInventory(targets []Target, hosts []inventory.Host) Diff {
	targetHosts := map[string]bool{}
	for _, t := range targets {
		for _, h := range t.Hosts {
			targetHosts[strings.ToLower(h)] = true
		}
	}

	var d Diff
	matched := map[string]bool{}
	for _, h := range hosts {
		name, addr := strings.ToLower(h.Name), strings.ToLower(h.Address)
		if targetHosts[name] || targetHosts[addr] {
			d.Covered = append(d.Covered, h.Name)
			matched[name], matched[addr] = true, true
			continue
		}
		d.Uncovered = append(d.Uncovered, h.Name)
	}
	seen := map[string]bool{}
	for _, t := range targets {
		for _, host := range t.Hosts {
			h := strings.ToLower(host)
			if matched[h] || seen[h] {
				continue
			}
			seen[h] = true
			d.Extra = append(d.Extra, host)
		}
	}
	return d
}
