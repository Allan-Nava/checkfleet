package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the root of checkfleet.yml.
type Config struct {
	TimeoutSeconds int          `yaml:"timeout_seconds"`
	Retries        int          `yaml:"retries"`          // retry checks with ERROR findings
	RetryBackoffMS int          `yaml:"retry_backoff_ms"` // base backoff (default 500 when retries>0)
	Checks         ChecksConfig `yaml:"checks"`
}

type ChecksConfig struct {
	Certs    *CertsConfig    `yaml:"certs"`
	HTTP     *HTTPConfig     `yaml:"http"`
	NATS     *NATSConfig     `yaml:"nats"`
	HAProxy  *HAProxyConfig  `yaml:"haproxy"`
	Stream   *StreamConfig   `yaml:"stream"`
	Patroni  *PatroniConfig  `yaml:"patroni"`
	Consul   *ConsulConfig   `yaml:"consul"`
	Postgres *PostgresConfig `yaml:"postgres"`
	DNS      *DNSConfig      `yaml:"dns"`
	Redis    *RedisConfig    `yaml:"redis"`
}

// CertsConfig configures the TLS certificate expiry check.
type CertsConfig struct {
	WarnDays int `yaml:"warn_days"`
	CritDays int `yaml:"crit_days"`
	// Default port for targets and inventory hosts without an explicit one.
	Port int `yaml:"port"`
	// Explicit host[:port] targets.
	Targets []string `yaml:"targets"`
	// Optional Ansible INI inventory: every host becomes a target on Port.
	AnsibleInventory string `yaml:"ansible_inventory"`
}

// NATSConfig configures the NATS JetStream cluster health check.
type NATSConfig struct {
	// Monitoring endpoints as host[:port]; Port applies when a target has none.
	Targets []string `yaml:"targets"`
	Port    int      `yaml:"port"`
	// Optional Ansible INI inventory: every host becomes a monitoring target.
	AnsibleInventory string `yaml:"ansible_inventory"`
	// Scheme for the monitoring endpoint (http or https). Default http.
	Scheme string `yaml:"scheme"`
	// Optional expected meta-leader (server_name); a mismatch is WARN.
	ExpectMetaLeader string `yaml:"expect_meta_leader"`
	// Optional expected peer set (server_name); unexpected peers are ghosts
	// (WARN), missing expected peers are BAD.
	ExpectPeers []string `yaml:"expect_peers"`
	// Raft peer lag thresholds (entries). WARN/BAD when a peer is at or above.
	LagWarn int `yaml:"lag_warn"`
	LagCrit int `yaml:"lag_crit"`
}

// HAProxyConfig configures the HAProxy backend/server health check.
type HAProxyConfig struct {
	// Stats endpoints as host[:port]; Port applies when a target has none.
	Targets []string `yaml:"targets"`
	Port    int      `yaml:"port"`
	// Scheme (http/https) and path of the CSV stats export.
	Scheme string `yaml:"scheme"`
	Path   string `yaml:"path"`
	// Optional Ansible INI inventory: every host becomes a stats target.
	AnsibleInventory string `yaml:"ansible_inventory"`
	// Optional WARN when a server/backend session usage reaches this percent
	// of its limit (scur/slim). 0 disables the check.
	SessionWarnPct int `yaml:"session_warn_pct"`
	// Optional HTTP basic auth. The password is read from the named env var —
	// never store it in the config file.
	AuthUser    string `yaml:"auth_user"`
	AuthPassEnv string `yaml:"auth_pass_env"`
}

// StreamConfig configures the HLS/DASH stream health check.
type StreamConfig struct {
	Targets []StreamTarget `yaml:"targets"`
}

type StreamTarget struct {
	// Manifest URL: an HLS .m3u8 (master or media) or a DASH .mpd.
	URL string `yaml:"url"`
	// Optional display label; defaults to the URL.
	Name string `yaml:"name"`
	// Expected minimum ladder size (variants/representations). 0 disables.
	MinVariants int `yaml:"min_variants"`
	// Expect a live stream: check live-edge freshness and warn if it's VOD.
	Live bool `yaml:"live"`
	// Live-edge age thresholds in seconds (WARN/BAD). Applied when Live is set.
	MaxAgeWarnSeconds int `yaml:"max_age_warn_seconds"`
	MaxAgeCritSeconds int `yaml:"max_age_crit_seconds"`
}

// PatroniConfig configures the Patroni cluster health check.
type PatroniConfig struct {
	// Patroni REST API endpoints as host[:port]; Port applies when a target
	// has none.
	Targets []string `yaml:"targets"`
	Port    int      `yaml:"port"`
	Scheme  string   `yaml:"scheme"`
	// Optional Ansible INI inventory: every host becomes an API target.
	AnsibleInventory string `yaml:"ansible_inventory"`
	// Replica lag thresholds in bytes (WARN/BAD).
	LagWarnBytes int64 `yaml:"lag_warn_bytes"`
	LagCritBytes int64 `yaml:"lag_crit_bytes"`
}

// ConsulConfig configures the Consul cluster health check.
type ConsulConfig struct {
	// Consul HTTP API endpoints as host[:port]; Port applies when a target has
	// none.
	Targets []string `yaml:"targets"`
	Port    int      `yaml:"port"`
	Scheme  string   `yaml:"scheme"`
	// Optional Ansible INI inventory: every host becomes an API target.
	AnsibleInventory string `yaml:"ansible_inventory"`
	// Optional expected number of raft peers; fewer than this is WARN.
	ExpectPeers int `yaml:"expect_peers"`
	// Optional ACL token, read from this env var (X-Consul-Token); never inline.
	TokenEnv string `yaml:"token_env"`
	// Optional KV keys that must exist; a missing key is BAD.
	KVKeys []string `yaml:"kv_keys"`
}

// PostgresConfig configures the PostgreSQL health check (read-only SQL).
type PostgresConfig struct {
	Targets []PostgresTarget `yaml:"targets"`
	// Replica lag thresholds in bytes (WARN/BAD).
	LagWarnBytes int64 `yaml:"lag_warn_bytes"`
	LagCritBytes int64 `yaml:"lag_crit_bytes"`
	// WARN when connections reach this percent of max_connections.
	ConnWarnPct int `yaml:"conn_warn_pct"`
	// Transaction-id age thresholds (WARN/BAD) for wraparound risk.
	WraparoundWarnAge int64 `yaml:"wraparound_warn_age"`
	WraparoundCritAge int64 `yaml:"wraparound_crit_age"`
	// Retained-WAL thresholds for inactive replication slots (WARN/BAD).
	SlotWarnBytes int64 `yaml:"slot_warn_bytes"`
	SlotCritBytes int64 `yaml:"slot_crit_bytes"`
}

type PostgresTarget struct {
	// Display label; defaults to the DSN host.
	Name string `yaml:"name"`
	// libpq DSN or URL, WITHOUT the password.
	DSN string `yaml:"dsn"`
	// Password read from this env var (never store it in the config).
	PasswordEnv string `yaml:"password_env"`
}

// DNSConfig configures the DNS resolution health check.
type DNSConfig struct {
	// Resolvers to query as host[:port] (default port 53). Empty → the system
	// resolvers from /etc/resolv.conf.
	Resolvers []string `yaml:"resolvers"`
	// WARN when any answer's TTL is below this many seconds. 0 disables.
	MinTTLSeconds uint32      `yaml:"min_ttl_seconds"`
	Targets       []DNSTarget `yaml:"targets"`
}

type DNSTarget struct {
	// Domain name to resolve.
	Name string `yaml:"name"`
	// Record type: A, AAAA, CNAME, TXT, NS, SOA. Default A.
	Type string `yaml:"type"`
	// Optional expected value set; a different answer is BAD (drift). For SOA
	// this is compared against the serial.
	Expect []string `yaml:"expect"`
}

// RedisConfig configures the Redis/Valkey health check.
type RedisConfig struct {
	// Endpoints as host[:port]; Port applies when a target has none.
	Targets []string `yaml:"targets"`
	Port    int      `yaml:"port"`
	// Optional Ansible INI inventory: every host becomes a target.
	AnsibleInventory string `yaml:"ansible_inventory"`
	// Optional TLS (rediss) and ACL auth. Password comes from the env var.
	TLS         bool   `yaml:"tls"`
	Username    string `yaml:"username"`
	PasswordEnv string `yaml:"password_env"`
	// WARN when used_memory reaches this percent of maxmemory (0 disables).
	MemWarnPct int `yaml:"mem_warn_pct"`
	// Replica offset lag thresholds in bytes (WARN/BAD).
	LagWarnBytes int64 `yaml:"lag_warn_bytes"`
	LagCritBytes int64 `yaml:"lag_crit_bytes"`
}

// HTTPConfig configures the HTTP probe check.
type HTTPConfig struct {
	Targets []HTTPTarget `yaml:"targets"`
}

type HTTPTarget struct {
	URL          string `yaml:"url"`
	ExpectStatus int    `yaml:"expect_status"`
	MaxLatencyMS int    `yaml:"max_latency_ms"`
	ExpectBody   string `yaml:"expect_body"`
}

// LoadConfig reads and validates checkfleet.yml, applying defaults.
func LoadConfig(path string) (*Config, error) {
	cfg, err := parseConfig(path)
	if err != nil {
		return nil, err
	}
	applyDefaults(cfg)
	return cfg, nil
}

// parseConfig reads and unmarshals a config file WITHOUT applying defaults, so
// callers can overlay one config on another before defaults kick in.
func parseConfig(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("config %s: %w", path, err)
	}
	return &cfg, nil
}

// applyDefaults fills in per-module defaults on a parsed config.
func applyDefaults(cfg *Config) {
	if cfg.TimeoutSeconds <= 0 {
		cfg.TimeoutSeconds = 30
	}
	if cfg.Retries > 0 && cfg.RetryBackoffMS <= 0 {
		cfg.RetryBackoffMS = 500
	}
	if c := cfg.Checks.Certs; c != nil {
		if c.WarnDays <= 0 {
			c.WarnDays = 30
		}
		if c.CritDays <= 0 {
			c.CritDays = 7
		}
		if c.Port <= 0 {
			c.Port = 443
		}
	}
	if h := cfg.Checks.HTTP; h != nil {
		for i := range h.Targets {
			if h.Targets[i].ExpectStatus == 0 {
				h.Targets[i].ExpectStatus = 200
			}
		}
	}
	if n := cfg.Checks.NATS; n != nil {
		if n.Port <= 0 {
			n.Port = 8222
		}
		if n.LagWarn <= 0 {
			n.LagWarn = 100
		}
		if n.LagCrit <= 0 {
			n.LagCrit = 1000
		}
	}
	if hp := cfg.Checks.HAProxy; hp != nil {
		if hp.Port <= 0 {
			hp.Port = 8404
		}
		if hp.Path == "" {
			hp.Path = "/stats;csv"
		}
	}
	if p := cfg.Checks.Patroni; p != nil {
		if p.Port <= 0 {
			p.Port = 8008
		}
		if p.LagWarnBytes <= 0 {
			p.LagWarnBytes = 32 << 20 // 32 MiB
		}
		if p.LagCritBytes <= 0 {
			p.LagCritBytes = 128 << 20 // 128 MiB
		}
	}
	if cn := cfg.Checks.Consul; cn != nil {
		if cn.Port <= 0 {
			cn.Port = 8500
		}
	}
	if r := cfg.Checks.Redis; r != nil {
		if r.Port <= 0 {
			r.Port = 6379
		}
		if r.MemWarnPct <= 0 {
			r.MemWarnPct = 80
		}
		if r.LagWarnBytes <= 0 {
			r.LagWarnBytes = 16 << 20 // 16 MiB
		}
		if r.LagCritBytes <= 0 {
			r.LagCritBytes = 128 << 20 // 128 MiB
		}
	}
	if pg := cfg.Checks.Postgres; pg != nil {
		if pg.LagWarnBytes <= 0 {
			pg.LagWarnBytes = 32 << 20 // 32 MiB
		}
		if pg.LagCritBytes <= 0 {
			pg.LagCritBytes = 128 << 20 // 128 MiB
		}
		if pg.ConnWarnPct <= 0 {
			pg.ConnWarnPct = 80
		}
		if pg.WraparoundWarnAge <= 0 {
			pg.WraparoundWarnAge = 1_500_000_000
		}
		if pg.WraparoundCritAge <= 0 {
			pg.WraparoundCritAge = 1_900_000_000
		}
		if pg.SlotWarnBytes <= 0 {
			pg.SlotWarnBytes = 512 << 20 // 512 MiB
		}
		if pg.SlotCritBytes <= 0 {
			pg.SlotCritBytes = 2 << 30 // 2 GiB
		}
	}
	if s := cfg.Checks.Stream; s != nil {
		for i := range s.Targets {
			if s.Targets[i].Live {
				if s.Targets[i].MaxAgeWarnSeconds <= 0 {
					s.Targets[i].MaxAgeWarnSeconds = 30
				}
				if s.Targets[i].MaxAgeCritSeconds <= 0 {
					s.Targets[i].MaxAgeCritSeconds = 60
				}
			}
		}
	}
}

// LoadConfigStack loads a base config and overlays a per-stack file
// (checkfleet.<stack>.yml next to the base), applying defaults after the
// merge. A module present in the stack replaces the base's module wholesale.
func LoadConfigStack(basePath, stack string) (*Config, error) {
	base, err := parseConfig(basePath)
	if err != nil {
		return nil, err
	}
	over, err := parseConfig(StackPath(basePath, stack))
	if err != nil {
		return nil, fmt.Errorf("stack %q: %w", stack, err)
	}
	base.overlay(over)
	applyDefaults(base)
	return base, nil
}

// overlay merges over on top of c: a set timeout and any non-nil module win.
func (c *Config) overlay(over *Config) {
	if over.TimeoutSeconds > 0 {
		c.TimeoutSeconds = over.TimeoutSeconds
	}
	o := over.Checks
	if o.Certs != nil {
		c.Checks.Certs = o.Certs
	}
	if o.HTTP != nil {
		c.Checks.HTTP = o.HTTP
	}
	if o.NATS != nil {
		c.Checks.NATS = o.NATS
	}
	if o.HAProxy != nil {
		c.Checks.HAProxy = o.HAProxy
	}
	if o.Stream != nil {
		c.Checks.Stream = o.Stream
	}
	if o.Patroni != nil {
		c.Checks.Patroni = o.Patroni
	}
	if o.Consul != nil {
		c.Checks.Consul = o.Consul
	}
	if o.Postgres != nil {
		c.Checks.Postgres = o.Postgres
	}
	if o.DNS != nil {
		c.Checks.DNS = o.DNS
	}
}

// StackPath derives the per-stack config path from the base path:
// "checkfleet.yml" + "prod" → "checkfleet.prod.yml".
func StackPath(basePath, stack string) string {
	ext := filepath.Ext(basePath) // ".yml"
	return strings.TrimSuffix(basePath, ext) + "." + stack + ext
}
