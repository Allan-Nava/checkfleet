package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the root of checkfleet.yml.
type Config struct {
	TimeoutSeconds int               `yaml:"timeout_seconds"`
	Retries        int               `yaml:"retries"`          // retry checks with ERROR findings
	RetryBackoffMS int               `yaml:"retry_backoff_ms"` // base backoff (default 500 when retries>0)
	MaxConcurrency int               `yaml:"max_concurrency"`  // cap on checks running at once (0 = unbounded, CF-116)
	Labels         map[string]string `yaml:"labels"`           // global labels attached to the outputs (CF-119)
	Checks         ChecksConfig      `yaml:"checks"`

	// Maintenance windows: findings matching an active window are muted or
	// downgraded so scheduled work doesn't page. See ApplyMaintenance.
	Maintenance []MaintenanceWindow `yaml:"maintenance"`

	// Runbooks attach operator hints (a procedure URL, a short remediation note)
	// to the findings that need attention. See ApplyRunbooks.
	Runbooks []RunbookRule `yaml:"runbooks"`

	// Per-module overrides of timeout/retries, keyed by module name. A field left
	// zero falls back to the global setting above. See registry.Jobs.
	ModuleOverrides map[string]ModuleOverride `yaml:"module_overrides"`
}

// ModuleOverride overrides run tuning for a single module (CF-84).
type ModuleOverride struct {
	TimeoutSeconds int `yaml:"timeout_seconds"`
	Retries        int `yaml:"retries"`
	RetryBackoffMS int `yaml:"retry_backoff_ms"`
}

// RunbookRule attaches a runbook URL and/or a remediation note to the findings
// it matches (CF-124). Matching mirrors MaintenanceWindow: globs on check and
// target, empty meaning "all". Rules are read in order and the first non-empty
// value wins per field, so a specific rule listed before a catch-all can supply
// the runbook while the catch-all still supplies the remediation.
//
// Operational text only — a URL and a short note. Never put a credential here:
// it is carried into every output, including the ones that leave the host.
type RunbookRule struct {
	Check       string `yaml:"check"`       // glob on the check name; "" = all
	Target      string `yaml:"target"`      // glob on the target; "" = all
	Runbook     string `yaml:"runbook"`     // URL of the procedure to follow
	Remediation string `yaml:"remediation"` // short "what to do" note
}

// MaintenanceWindow suppresses (or downgrades) findings during a time range.
type MaintenanceWindow struct {
	Check  string `yaml:"check"`  // glob on the check name; "" = all
	Target string `yaml:"target"` // glob on the target; "" = all
	From   string `yaml:"from"`   // RFC3339 start (inclusive); "" = unbounded
	To     string `yaml:"to"`     // RFC3339 end (inclusive); "" = unbounded
	Action string `yaml:"action"` // "mute" (drop, default) or "warn" (cap at WARN)
	// Recurring window: a daily local clock range "HH:MM-HH:MM" (wraps past
	// midnight), optionally restricted to Weekdays. When set, From/To still bound
	// the overall validity. Empty Daily = one-shot window (From/To only).
	Daily    string   `yaml:"daily"`
	Weekdays []string `yaml:"weekdays"` // e.g. [Sat, Sun]; empty = every day
}

type ChecksConfig struct {
	Certs         *CertsConfig         `yaml:"certs"`
	HTTP          *HTTPConfig          `yaml:"http"`
	NATS          *NATSConfig          `yaml:"nats"`
	HAProxy       *HAProxyConfig       `yaml:"haproxy"`
	Stream        *StreamConfig        `yaml:"stream"`
	Patroni       *PatroniConfig       `yaml:"patroni"`
	Consul        *ConsulConfig        `yaml:"consul"`
	Postgres      *PostgresConfig      `yaml:"postgres"`
	DNS           *DNSConfig           `yaml:"dns"`
	Redis         *RedisConfig         `yaml:"redis"`
	Keycloak      *KeycloakConfig      `yaml:"keycloak"`
	TCP           *TCPConfig           `yaml:"tcp"`
	TLS           *TLSConfig           `yaml:"tls"`
	NTP           *NTPConfig           `yaml:"ntp"`
	RabbitMQ      *RabbitMQConfig      `yaml:"rabbitmq"`
	GRPC          *GRPCConfig          `yaml:"grpc"`
	LDAP          *LDAPConfig          `yaml:"ldap"`
	Kafka         *KafkaConfig         `yaml:"kafka"`
	Ingest        *IngestConfig        `yaml:"ingest"`
	S3            *S3Config            `yaml:"s3"`
	SMTP          *SMTPConfig          `yaml:"smtp"`
	Elasticsearch *ElasticsearchConfig `yaml:"elasticsearch"`
	MongoDB       *MongoDBConfig       `yaml:"mongodb"`
	MySQL         *MySQLConfig         `yaml:"mysql"`
	Etcd          *EtcdConfig          `yaml:"etcd"`
	ClickHouse    *ClickHouseConfig    `yaml:"clickhouse"`
	Vault         *VaultConfig         `yaml:"vault"`
	Memcached     *MemcachedConfig     `yaml:"memcached"`
	Cassandra     *CassandraConfig     `yaml:"cassandra"`
}

// CassandraConfig configures the Cassandra/ScyllaDB reachability check.
type CassandraConfig struct {
	Targets []CassandraTarget `yaml:"targets"`
	// ExpectNodes is how many nodes must accept CQL for the cluster to be
	// healthy (0 = expect them all). Fewer than this is BAD, like etcd's
	// expect_members.
	ExpectNodes int `yaml:"expect_nodes"`
}

// CassandraTarget is one node to probe over the CQL native protocol.
type CassandraTarget struct {
	Name string `yaml:"name"`
	// host[:port]; default CQL port 9042.
	Address      string `yaml:"address"`
	MaxLatencyMS int    `yaml:"max_latency_ms"`
}

// MemcachedConfig configures the memcached check (text protocol).
type MemcachedConfig struct {
	// Endpoints as host[:port]; Port applies when a target has none.
	Targets    []string `yaml:"targets"`
	Port       int      `yaml:"port"`         // default 11211
	MemWarnPct int      `yaml:"mem_warn_pct"` // default 90
	// EvictionsWarn is an absolute count of evictions since the server started
	// (memcached exposes no rate), so it has no sensible default: 0 disables
	// the threshold and the counter is only reported as a metric.
	EvictionsWarn int64 `yaml:"evictions_warn"`
}

// VaultConfig configures the HashiCorp Vault check (HTTP API).
type VaultConfig struct {
	Targets []VaultTarget `yaml:"targets"`
}

// VaultTarget is one Vault endpoint.
type VaultTarget struct {
	Name string `yaml:"name"`
	// API address including scheme, e.g. https://vault-01:8200.
	URL string `yaml:"url"`
	// Optional token (from env) for authenticated endpoints; seal-status and
	// health need none.
	TokenEnv string `yaml:"token_env"`
	Insecure bool   `yaml:"insecure_skip_verify"`
}

// ClickHouseConfig configures the ClickHouse check (HTTP interface).
type ClickHouseConfig struct {
	Targets          []ClickHouseTarget `yaml:"targets"`
	DelayWarnSeconds int                `yaml:"delay_warn_seconds"` // default 30
	DelayCritSeconds int                `yaml:"delay_crit_seconds"` // default 300
}

// ClickHouseTarget is one ClickHouse HTTP endpoint.
type ClickHouseTarget struct {
	Name string `yaml:"name"`
	// HTTP endpoint including scheme, e.g. http://ch-01:8123.
	URL string `yaml:"url"`
	// Optional auth; password from env, never inline.
	Username    string `yaml:"username"`
	PasswordEnv string `yaml:"password_env"`
	Insecure    bool   `yaml:"insecure_skip_verify"`
}

// EtcdConfig configures the etcd v3 cluster check (HTTP JSON gateway).
type EtcdConfig struct {
	Targets       []EtcdTarget `yaml:"targets"`
	ExpectMembers int          `yaml:"expect_members"` // BAD if fewer members (0 disables)
}

// EtcdTarget is one etcd client endpoint.
type EtcdTarget struct {
	Name string `yaml:"name"`
	// Client URL including scheme, e.g. https://etcd-01:2379.
	URL string `yaml:"url"`
	// Optional token auth; password from env, never inline.
	Username    string `yaml:"username"`
	PasswordEnv string `yaml:"password_env"`
	// Skip TLS verification (self-signed clusters).
	Insecure bool `yaml:"insecure_skip_verify"`
}

// MySQLConfig configures the MySQL/MariaDB check. Read-only; put the password in
// the DSN via ${ENV} interpolation, never inline.
type MySQLConfig struct {
	Targets        []MySQLTarget `yaml:"targets"`
	ConnWarnPct    int           `yaml:"conn_warn_pct"`    // default 80
	LagWarnSeconds int           `yaml:"lag_warn_seconds"` // default 10
	LagCritSeconds int           `yaml:"lag_crit_seconds"` // default 60
}

// MySQLTarget is one MySQL/MariaDB server.
type MySQLTarget struct {
	Name string `yaml:"name"`
	// go-sql-driver DSN, e.g. monitor:${MYSQL_PW}@tcp(db:3306)/.
	DSN string `yaml:"dsn"`
}

// MongoDBConfig configures the MongoDB check. Credentials come from env; the
// check is read-only (serverStatus + replSetGetStatus).
type MongoDBConfig struct {
	Targets        []MongoDBTarget `yaml:"targets"`
	ConnWarnPct    int             `yaml:"conn_warn_pct"`    // default 80
	LagWarnSeconds int             `yaml:"lag_warn_seconds"` // default 10
	LagCritSeconds int             `yaml:"lag_crit_seconds"` // default 60
}

// MongoDBTarget is one MongoDB deployment (any member answers replSetGetStatus).
type MongoDBTarget struct {
	Name string `yaml:"name"`
	// Connection URI without credentials, e.g. mongodb://host:27017.
	URI string `yaml:"uri"`
	// Optional auth; password from env, never inline.
	Username    string `yaml:"username"`
	PasswordEnv string `yaml:"password_env"`
	AuthSource  string `yaml:"auth_source"` // default admin
}

// ElasticsearchConfig configures the Elasticsearch/OpenSearch cluster check.
// Disk watermark thresholds are cluster-wide; credentials come from env.
type ElasticsearchConfig struct {
	Targets     []ElasticsearchTarget `yaml:"targets"`
	DiskWarnPct int                   `yaml:"disk_warn_pct"` // default 85 (ES low watermark)
	DiskCritPct int                   `yaml:"disk_crit_pct"` // default 90 (ES high watermark)
	// Client certificate for mTLS (CF-183); empty leaves the handshake unchanged.
	ClientTLS ClientTLS `yaml:",inline"`
}

// ElasticsearchTarget is one cluster endpoint (any node answers cluster-wide).
type ElasticsearchTarget struct {
	Name string `yaml:"name"`
	// Base URL including scheme, e.g. https://es.example.com:9200.
	URL string `yaml:"url"`
	// HTTP basic auth (password from env) or an API key from env.
	Username    string `yaml:"username"`
	PasswordEnv string `yaml:"password_env"`
	APIKeyEnv   string `yaml:"api_key_env"`
	// Skip TLS verification (self-signed clusters).
	Insecure bool `yaml:"insecure_skip_verify"`
	// BAD when the cluster reports fewer nodes than this (0 disables).
	ExpectNodes int `yaml:"expect_nodes"`
}

// SMTPConfig configures the SMTP relay reachability check. It never sends mail;
// warn_days/crit_days apply to the relay certificate (implicit TLS or STARTTLS).
type SMTPConfig struct {
	Targets  []SMTPTarget `yaml:"targets"`
	WarnDays int          `yaml:"warn_days"`
	CritDays int          `yaml:"crit_days"`
	// Client certificate for mTLS (CF-183); empty leaves the handshake unchanged.
	ClientTLS ClientTLS `yaml:",inline"`
}

// SMTPTarget is one SMTP relay to probe.
type SMTPTarget struct {
	Name string `yaml:"name"`
	// host[:port]; default port 25, or 465 when tls is set.
	Address string `yaml:"address"`
	// Implicit TLS (smtps) — wrap the connection immediately.
	TLS bool `yaml:"tls"`
	// Require STARTTLS on a plain connection: BAD if not offered or it fails.
	StartTLS bool `yaml:"starttls"`
	// Optional substring the 220 greeting must contain.
	ExpectBanner string `yaml:"expect_banner"`
	// Optional WARN when the connect takes longer than this.
	MaxLatencyMS int `yaml:"max_latency_ms"`
}

// IngestConfig configures the ingest (RTMP/SRT) reachability check.
type IngestConfig struct {
	Targets []IngestTarget `yaml:"targets"`
}

// IngestTarget is one streaming ingest endpoint.
type IngestTarget struct {
	Name         string `yaml:"name"`
	Address      string `yaml:"address"`  // host:port
	Protocol     string `yaml:"protocol"` // "rtmp" (default) or "srt"
	MaxLatencyMS int    `yaml:"max_latency_ms"`
}

// S3Config configures the S3/object-storage check. Credentials come from env.
type S3Config struct {
	Targets []S3Target `yaml:"targets"`
}

// S3Target is one bucket (optionally with a sentinel object to check freshness).
type S3Target struct {
	Name string `yaml:"name"`
	// Endpoint base URL, e.g. https://s3.example.com or https://minio:9000.
	Endpoint string `yaml:"endpoint"`
	Bucket   string `yaml:"bucket"`
	Region   string `yaml:"region"`
	// Optional sentinel object key; if set, it must exist and be fresh.
	Object string `yaml:"object"`
	// Object considered stale (WARN) when older than this many seconds.
	MaxAgeWarnSeconds int `yaml:"max_age_warn_seconds"`
	// Env vars holding the access key id / secret. Never in config.
	AccessKeyEnv string `yaml:"access_key_env"`
	SecretKeyEnv string `yaml:"secret_key_env"`
	// Use path-style addressing (MinIO/Ceph); default virtual-hosted.
	PathStyle bool `yaml:"path_style"`
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

// KeycloakConfig configures the Keycloak health check.
type KeycloakConfig struct {
	// Base URL (scheme + host [+ path prefix like /auth]), no trailing slash.
	BaseURL string `yaml:"base_url"`
	// Optional health endpoint (e.g. https://auth:9000/health/ready); checked
	// only when set (Keycloak often serves health on the management port).
	HealthURL string `yaml:"health_url"`
	// Realms to verify via their OIDC discovery document.
	Realms []string `yaml:"realms"`
}

// TCPConfig configures the generic TCP reachability check.
type TCPConfig struct {
	Targets []TCPTarget `yaml:"targets"`
	// Client certificate for mTLS (CF-183); empty leaves the handshake unchanged.
	ClientTLS ClientTLS `yaml:",inline"`
}

type TCPTarget struct {
	// Optional display label; defaults to Address.
	Name string `yaml:"name"`
	// host:port to connect to. Required.
	Address string `yaml:"address"`
	// Optional TLS handshake instead of a plain TCP connect.
	TLS bool `yaml:"tls"`
	// Optional substring the server banner must contain (first bytes read).
	ExpectBanner string `yaml:"expect_banner"`
	// Optional WARN when the connect takes longer than this.
	MaxLatencyMS int `yaml:"max_latency_ms"`
}

// TLSConfig configures the deep TLS check (chain validity, expiry, protocol).
type TLSConfig struct {
	Targets          []string `yaml:"targets"`
	Port             int      `yaml:"port"`
	WarnDays         int      `yaml:"warn_days"`
	CritDays         int      `yaml:"crit_days"`
	AnsibleInventory string   `yaml:"ansible_inventory"`
}

// NTPConfig configures the NTP clock-offset check.
type NTPConfig struct {
	Targets      []string `yaml:"targets"`
	Port         int      `yaml:"port"`
	OffsetWarnMS int      `yaml:"offset_warn_ms"`
	OffsetCritMS int      `yaml:"offset_crit_ms"`
}

// RabbitMQConfig configures the RabbitMQ management-API health check.
type RabbitMQConfig struct {
	// Management API endpoints as host[:port]; Port applies when none.
	Targets []string `yaml:"targets"`
	Port    int      `yaml:"port"`
	Scheme  string   `yaml:"scheme"`
	// HTTP basic auth. Password from the env var; never inline.
	Username    string `yaml:"username"`
	PasswordEnv string `yaml:"password_env"`
	// Queue depth thresholds (messages ready+unacked).
	QueueWarnDepth int `yaml:"queue_warn_depth"`
	QueueCritDepth int `yaml:"queue_crit_depth"`
}

// GRPCConfig configures the gRPC health-checking-protocol check (TLS/h2 only).
type GRPCConfig struct {
	Targets []GRPCTarget `yaml:"targets"`
	// Client certificate for mTLS (CF-183); empty leaves the handshake unchanged.
	ClientTLS ClientTLS `yaml:",inline"`
}

type GRPCTarget struct {
	// Optional display label; defaults to Address (+ service).
	Name string `yaml:"name"`
	// host:port of the gRPC (TLS) endpoint. Required.
	Address string `yaml:"address"`
	// Optional gRPC service name to check; empty = whole-server health.
	Service string `yaml:"service"`
	// Skip TLS certificate verification (internal self-signed endpoints).
	InsecureSkipVerify bool `yaml:"insecure_skip_verify"`
}

// LDAPConfig configures the LDAP bind + search check.
type LDAPConfig struct {
	Targets []LDAPTarget `yaml:"targets"`
}

type LDAPTarget struct {
	// Optional display label; defaults to URL.
	Name string `yaml:"name"`
	// ldap://host:389 or ldaps://host:636. Required.
	URL string `yaml:"url"`
	// Optional StartTLS on a plain ldap:// connection.
	StartTLS           bool `yaml:"start_tls"`
	InsecureSkipVerify bool `yaml:"insecure_skip_verify"`
	// Optional bind; empty BindDN = anonymous. Password from the env var.
	BindDN      string `yaml:"bind_dn"`
	PasswordEnv string `yaml:"password_env"`
	// Optional sanity search: at least MinEntries under BaseDN matching Filter.
	BaseDN     string `yaml:"base_dn"`
	Filter     string `yaml:"filter"`
	MinEntries int    `yaml:"min_entries"`
}

// KafkaConfig configures the Kafka cluster health check.
type KafkaConfig struct {
	Brokers []string `yaml:"brokers"`
	TLS     bool     `yaml:"tls"`
	// Optional SASL: mechanism plain|scram-sha-256|scram-sha-512; password from env.
	SASLUser        string `yaml:"sasl_user"`
	SASLMechanism   string `yaml:"sasl_mechanism"`
	SASLPasswordEnv string `yaml:"sasl_password_env"`
	// Client certificate for mTLS (CF-183); empty leaves the handshake unchanged.
	ClientTLS ClientTLS `yaml:",inline"`
	// Optional expected broker count; fewer is WARN.
	ExpectBrokers int `yaml:"expect_brokers"`
	// Consumer groups whose lag to check.
	Groups  []string `yaml:"groups"`
	LagWarn int64    `yaml:"lag_warn"`
	LagCrit int64    `yaml:"lag_crit"`
}

// HTTPConfig configures the HTTP probe check.
type HTTPConfig struct {
	Targets []HTTPTarget `yaml:"targets"`
	// Client certificate for mTLS (CF-183); empty leaves the handshake unchanged.
	ClientTLS ClientTLS `yaml:",inline"`
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

// LoadBytes parses config from raw YAML bytes (with ${...} interpolation and
// defaults applied), without touching disk. Used by the desktop config editor
// to validate unsaved text.
func LoadBytes(raw []byte) (*Config, error) {
	raw, err := expandVars(raw)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	applyDefaults(&cfg)
	return &cfg, nil
}

// parseConfig reads and unmarshals a config file WITHOUT applying defaults, so
// callers can overlay one config on another before defaults kick in. It resolves
// any `include:` files first (CF-115), deep-merging them under this file, then
// applies ${...} interpolation to the merged result.
func parseConfig(path string) (*Config, error) {
	merged, err := loadMergedMap(path, map[string]bool{})
	if err != nil {
		return nil, err
	}
	raw, err := yaml.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("config %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("config %s: %w", path, err)
	}
	return &cfg, nil
}

// loadMergedMap reads a config file into a generic map and resolves its
// `include:` list (CF-115): each included file (a path or list of paths,
// resolved relative to THIS file's directory) is loaded and deep-merged UNDER
// the current file, so the file that does the including always wins. Includes
// apply in listed order (a later include wins over an earlier one). `visiting`
// tracks the in-progress include chain by absolute path so a cycle is reported
// instead of recursing forever; a diamond (the same file reached two ways) is
// fine. Interpolation is deferred to the caller so it runs once on the merged
// whole.
func loadMergedMap(path string, visiting map[string]bool) (map[string]any, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	if visiting[abs] {
		return nil, fmt.Errorf("config: include cycle at %s", path)
	}
	visiting[abs] = true
	defer delete(visiting, abs)

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	// Interpolate ${...} per file on the original bytes (its own formatting),
	// before the map round-trip, so a merged value can't get re-quoted.
	raw, err = expandVars(raw)
	if err != nil {
		return nil, fmt.Errorf("config %s: %w", path, err)
	}
	var m map[string]any
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("config %s: %w", path, err)
	}
	if m == nil {
		m = map[string]any{}
	}

	includes := includePaths(m["include"])
	delete(m, "include")

	base := map[string]any{}
	dir := filepath.Dir(path)
	for _, inc := range includes {
		if !filepath.IsAbs(inc) {
			inc = filepath.Join(dir, inc)
		}
		files, err := expandInclude(inc)
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			sub, err := loadMergedMap(f, visiting)
			if err != nil {
				return nil, err
			}
			deepMerge(base, sub)
		}
	}
	deepMerge(base, m) // the including file wins over everything it includes
	return base, nil
}

// expandInclude resolves one include entry to the files to merge: a plain file
// is itself; a directory (a conf.d/ style drop-in) expands to its *.yml/*.yaml
// entries in sorted order, so drop-ins apply predictably.
func expandInclude(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("config include: %w", err)
	}
	if !info.IsDir() {
		return []string{path}, nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("config include dir %s: %w", path, err)
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if ext := strings.ToLower(filepath.Ext(e.Name())); ext == ".yml" || ext == ".yaml" {
			files = append(files, filepath.Join(path, e.Name()))
		}
	}
	sort.Strings(files) // ReadDir is already sorted, but be explicit
	return files, nil
}

// includePaths coerces the `include` field into a list of paths, accepting a
// single string or a list of strings (anything else is ignored).
func includePaths(v any) []string {
	switch t := v.(type) {
	case string:
		if t == "" {
			return nil
		}
		return []string{t}
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// deepMerge overlays src onto dst in place: nested maps merge key-by-key, so two
// files can each contribute different modules under `checks:`; any other value
// (scalar or list — e.g. a module's `targets:`) replaces wholesale, so
// redefining a module overrides it rather than appending.
func deepMerge(dst, src map[string]any) {
	for k, sv := range src {
		if sm, ok := sv.(map[string]any); ok {
			if dm, ok := dst[k].(map[string]any); ok {
				deepMerge(dm, sm)
				continue
			}
		}
		dst[k] = sv
	}
}

// varPattern matches ${...} interpolation tokens in a config file.
var varPattern = regexp.MustCompile(`\$\{([^}]*)\}`)

// expandVars interpolates ${...} tokens in the raw config before parsing:
//
//	${VAR}            environment variable VAR (empty if unset)
//	${VAR:-default}   VAR, or default when unset/empty
//	${file:/path}     the trimmed contents of a file (Docker/K8s secrets)
//
// A missing secret file is an error; unknown env vars expand to empty. Use
// $${ to emit a literal ${.
func expandVars(raw []byte) ([]byte, error) {
	src := strings.ReplaceAll(string(raw), "$${", "\x00")
	var firstErr error
	out := varPattern.ReplaceAllStringFunc(src, func(tok string) string {
		inner := tok[2 : len(tok)-1] // strip ${ and }
		switch {
		case strings.HasPrefix(inner, "file:"):
			b, err := os.ReadFile(strings.TrimPrefix(inner, "file:"))
			if err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("secret file: %w", err)
				}
				return ""
			}
			return strings.TrimSpace(string(b))
		case strings.Contains(inner, ":-"):
			name, def, _ := strings.Cut(inner, ":-")
			if v := os.Getenv(name); v != "" {
				return v
			}
			return def
		default:
			return os.Getenv(inner)
		}
	})
	if firstErr != nil {
		return nil, firstErr
	}
	return []byte(strings.ReplaceAll(out, "\x00", "${")), nil
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
	if t := cfg.Checks.TLS; t != nil {
		if t.Port <= 0 {
			t.Port = 443
		}
		if t.WarnDays <= 0 {
			t.WarnDays = 30
		}
		if t.CritDays <= 0 {
			t.CritDays = 7
		}
	}
	if k := cfg.Checks.Kafka; k != nil {
		if k.LagWarn <= 0 {
			k.LagWarn = 1000
		}
		if k.LagCrit <= 0 {
			k.LagCrit = 100000
		}
	}
	if l := cfg.Checks.LDAP; l != nil {
		for i := range l.Targets {
			if l.Targets[i].Filter == "" {
				l.Targets[i].Filter = "(objectClass=*)"
			}
			if l.Targets[i].BaseDN != "" && l.Targets[i].MinEntries <= 0 {
				l.Targets[i].MinEntries = 1
			}
		}
	}
	if rb := cfg.Checks.RabbitMQ; rb != nil {
		if rb.Port <= 0 {
			rb.Port = 15672
		}
		if rb.Username == "" {
			rb.Username = "guest"
		}
		if rb.QueueWarnDepth <= 0 {
			rb.QueueWarnDepth = 1000
		}
		if rb.QueueCritDepth <= 0 {
			rb.QueueCritDepth = 50000
		}
	}
	if n := cfg.Checks.NTP; n != nil {
		if n.Port <= 0 {
			n.Port = 123
		}
		if n.OffsetWarnMS <= 0 {
			n.OffsetWarnMS = 100
		}
		if n.OffsetCritMS <= 0 {
			n.OffsetCritMS = 1000
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
	if sm := cfg.Checks.SMTP; sm != nil {
		if sm.WarnDays <= 0 {
			sm.WarnDays = 30
		}
		if sm.CritDays <= 0 {
			sm.CritDays = 7
		}
	}
	if es := cfg.Checks.Elasticsearch; es != nil {
		if es.DiskWarnPct <= 0 {
			es.DiskWarnPct = 85
		}
		if es.DiskCritPct <= 0 {
			es.DiskCritPct = 90
		}
	}
	if mg := cfg.Checks.MongoDB; mg != nil {
		if mg.ConnWarnPct <= 0 {
			mg.ConnWarnPct = 80
		}
		if mg.LagWarnSeconds <= 0 {
			mg.LagWarnSeconds = 10
		}
		if mg.LagCritSeconds <= 0 {
			mg.LagCritSeconds = 60
		}
	}
	if my := cfg.Checks.MySQL; my != nil {
		if my.ConnWarnPct <= 0 {
			my.ConnWarnPct = 80
		}
		if my.LagWarnSeconds <= 0 {
			my.LagWarnSeconds = 10
		}
		if my.LagCritSeconds <= 0 {
			my.LagCritSeconds = 60
		}
	}
	if ch := cfg.Checks.ClickHouse; ch != nil {
		if ch.DelayWarnSeconds <= 0 {
			ch.DelayWarnSeconds = 30
		}
		if ch.DelayCritSeconds <= 0 {
			ch.DelayCritSeconds = 300
		}
	}
	if mc := cfg.Checks.Memcached; mc != nil {
		if mc.Port <= 0 {
			mc.Port = 11211
		}
		if mc.MemWarnPct <= 0 {
			mc.MemWarnPct = 90
		}
	}
}

// LoadConfigStack loads a base config and overlays a single per-stack file.
// It is LoadConfigStacks with one stack, kept for callers that pass just one.
func LoadConfigStack(basePath, stack string) (*Config, error) {
	return LoadConfigStacks(basePath, []string{stack})
}

// LoadConfigStacks loads a base config and overlays per-stack files in order
// (CF-117): checkfleet.<stack>.yml next to the base, for each stack, applying
// defaults once at the end. Overlays compose left-to-right so the LAST stack
// wins — e.g. --stack region,env layers env on top of region on top of base.
// A module present in a stack replaces that module wholesale (it gets its own
// defaults again); a module the stack leaves out is inherited. Each stack file
// resolves its own includes (CF-115) before overlaying.
func LoadConfigStacks(basePath string, stacks []string) (*Config, error) {
	base, err := parseConfig(basePath)
	if err != nil {
		return nil, err
	}
	for _, s := range stacks {
		if s == "" {
			continue
		}
		over, err := parseConfig(StackPath(basePath, s))
		if err != nil {
			return nil, fmt.Errorf("stack %q: %w", s, err)
		}
		base.overlay(over)
	}
	applyDefaults(base)
	return base, nil
}

// overlay merges over on top of c: a set timeout wins, and any module the stack
// defines replaces c's module wholesale. The module copy is generic (reflection
// over the Checks struct) so every module — present and future — is covered;
// the earlier hand-listed version silently ignored modules added after it.
func (c *Config) overlay(over *Config) {
	if over.TimeoutSeconds > 0 {
		c.TimeoutSeconds = over.TimeoutSeconds
	}
	oc := reflect.ValueOf(&over.Checks).Elem()
	cc := reflect.ValueOf(&c.Checks).Elem()
	for i := 0; i < oc.NumField(); i++ {
		f := oc.Field(i)
		// Every module is a pointer field; a non-nil one replaces the base's.
		if f.Kind() == reflect.Pointer && !f.IsNil() {
			cc.Field(i).Set(f)
		}
	}
}

// StackPath derives the per-stack config path from the base path:
// "checkfleet.yml" + "prod" → "checkfleet.prod.yml".
func StackPath(basePath, stack string) string {
	ext := filepath.Ext(basePath) // ".yml"
	return strings.TrimSuffix(basePath, ext) + "." + stack + ext
}
