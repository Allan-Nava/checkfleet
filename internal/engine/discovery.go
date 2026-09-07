package engine

// Discovery says where a module's targets come from, beyond the ones written
// out in `targets:` (CF-179).
//
// A real fleet already keeps this list somewhere — a service catalog, a set of
// SRV records — and retyping it into checkfleet.yml means maintaining it twice
// and discovering the drift when a check quietly stops covering a host.
//
// Spliced inline into the module configs that support it, so `ansible_inventory`
// keeps the exact key and meaning it had: a config written before this still
// works unchanged, which the compatibility contract requires.
type Discovery struct {
	// AnsibleInventory is a path to an INI inventory (file or directory).
	AnsibleInventory string `yaml:"ansible_inventory"`
	// ConsulService pulls the healthy instances of a service from a Consul
	// catalog.
	ConsulService *ConsulService `yaml:"consul_service"`
	// DNSSRV pulls hosts from SRV records, one lookup per entry.
	DNSSRV []SRVLookup `yaml:"dns_srv"`
}

// SRVLookup is one SRV record lookup.
type SRVLookup struct {
	// Name is the full record name, e.g. _nats._tcp.service.consul.
	Name string `yaml:"name"`
	// Resolver optionally sends the query to a specific nameserver
	// (host[:port], default port 53) instead of the system one. Names served
	// only by a service-discovery resolver — Consul's DNS on :8600, a
	// Kubernetes CoreDNS — are unreachable otherwise.
	Resolver string `yaml:"resolver"`
	// KeepPort appends the port the record advertises, default true. A
	// discovery source that knows the port and drops it throws away the useful
	// half of the answer; set false when the module supplies its own (certs on
	// 443 discovered from an SRV pointing at 8443).
	KeepPort *bool `yaml:"keep_port"`
}

// Set reports whether any source was configured.
func (d Discovery) Set() bool {
	return d.AnsibleInventory != "" || d.ConsulService != nil || len(d.DNSSRV) > 0
}

// ConsulService is a lookup against a Consul catalog.
type ConsulService struct {
	// Address of a Consul agent, host[:port] (default port 8500).
	Address string `yaml:"address"`
	Scheme  string `yaml:"scheme"`  // http (default) or https
	Service string `yaml:"service"` // the service name to look up
	// Tag optionally narrows the lookup to instances carrying it.
	Tag string `yaml:"tag"`
	// TokenEnv names the env var holding an ACL token, never the token.
	TokenEnv string `yaml:"token_env"`
	// OnlyHealthy uses the health endpoint instead of the raw catalog, so
	// instances failing their own checks are left out. Default true: a target
	// list that includes known-dead nodes turns one outage into two alerts.
	OnlyHealthy *bool `yaml:"only_healthy"`
	// KeepPort appends the service port to each address, default true.
	KeepPort *bool `yaml:"keep_port"`
}

// Healthy resolves the OnlyHealthy default (true when unset).
func (c ConsulService) Healthy() bool { return c.OnlyHealthy == nil || *c.OnlyHealthy }

// Port resolves the KeepPort default (true when unset).
func (c ConsulService) Port() bool { return c.KeepPort == nil || *c.KeepPort }

// Port resolves the KeepPort default (true when unset).
func (s SRVLookup) Port() bool { return s.KeepPort == nil || *s.KeepPort }
