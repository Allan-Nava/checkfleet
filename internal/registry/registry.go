// Package registry is the single source of truth for which check modules exist
// and how each is built from the typed config. Both the CLI (cmd/checkfleet)
// and the desktop app (desktop/) wire their checks through here so the module
// list lives in exactly one place — adding a module means editing Modules only.
package registry

import (
	"time"

	"github.com/Allan-Nava/checkfleet/internal/checks/certs"
	"github.com/Allan-Nava/checkfleet/internal/checks/cassandra"
	"github.com/Allan-Nava/checkfleet/internal/checks/clickhouse"
	"github.com/Allan-Nava/checkfleet/internal/checks/consul"
	"github.com/Allan-Nava/checkfleet/internal/checks/dns"
	"github.com/Allan-Nava/checkfleet/internal/checks/elasticsearch"
	"github.com/Allan-Nava/checkfleet/internal/checks/etcd"
	"github.com/Allan-Nava/checkfleet/internal/checks/grpccheck"
	"github.com/Allan-Nava/checkfleet/internal/checks/haproxy"
	"github.com/Allan-Nava/checkfleet/internal/checks/httpcheck"
	"github.com/Allan-Nava/checkfleet/internal/checks/ingest"
	"github.com/Allan-Nava/checkfleet/internal/checks/kafka"
	"github.com/Allan-Nava/checkfleet/internal/checks/keycloak"
	"github.com/Allan-Nava/checkfleet/internal/checks/ldapcheck"
	"github.com/Allan-Nava/checkfleet/internal/checks/memcached"
	"github.com/Allan-Nava/checkfleet/internal/checks/mongodb"
	"github.com/Allan-Nava/checkfleet/internal/checks/mysql"
	"github.com/Allan-Nava/checkfleet/internal/checks/nats"
	"github.com/Allan-Nava/checkfleet/internal/checks/ntp"
	"github.com/Allan-Nava/checkfleet/internal/checks/patroni"
	"github.com/Allan-Nava/checkfleet/internal/checks/postgres"
	"github.com/Allan-Nava/checkfleet/internal/checks/rabbitmq"
	"github.com/Allan-Nava/checkfleet/internal/checks/redis"
	"github.com/Allan-Nava/checkfleet/internal/checks/s3"
	"github.com/Allan-Nava/checkfleet/internal/checks/smtp"
	"github.com/Allan-Nava/checkfleet/internal/checks/stream"
	"github.com/Allan-Nava/checkfleet/internal/checks/tcp"
	"github.com/Allan-Nava/checkfleet/internal/checks/tlscheck"
	"github.com/Allan-Nava/checkfleet/internal/checks/vault"
	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// Spec ties a module name to whether it is configured and how to build it.
type Spec struct {
	Name       string
	Configured bool
	Build      func() engine.Check
}

// Modules is the registry of every check module, in a stable order.
func Modules(cfg *engine.Config) []Spec {
	c := cfg.Checks
	return []Spec{
		{"certs", c.Certs != nil, func() engine.Check { return certs.New(*c.Certs) }},
		{"http", c.HTTP != nil, func() engine.Check { return httpcheck.New(*c.HTTP) }},
		{"nats", c.NATS != nil, func() engine.Check { return nats.New(*c.NATS) }},
		{"haproxy", c.HAProxy != nil, func() engine.Check { return haproxy.New(*c.HAProxy) }},
		{"stream", c.Stream != nil, func() engine.Check { return stream.New(*c.Stream) }},
		{"patroni", c.Patroni != nil, func() engine.Check { return patroni.New(*c.Patroni) }},
		{"consul", c.Consul != nil, func() engine.Check { return consul.New(*c.Consul) }},
		{"postgres", c.Postgres != nil, func() engine.Check { return postgres.New(*c.Postgres) }},
		{"dns", c.DNS != nil, func() engine.Check { return dns.New(*c.DNS) }},
		{"redis", c.Redis != nil, func() engine.Check { return redis.New(*c.Redis) }},
		{"keycloak", c.Keycloak != nil, func() engine.Check { return keycloak.New(*c.Keycloak) }},
		{"tcp", c.TCP != nil, func() engine.Check { return tcp.New(*c.TCP) }},
		{"tls", c.TLS != nil, func() engine.Check { return tlscheck.New(*c.TLS) }},
		{"ntp", c.NTP != nil, func() engine.Check { return ntp.New(*c.NTP) }},
		{"rabbitmq", c.RabbitMQ != nil, func() engine.Check { return rabbitmq.New(*c.RabbitMQ) }},
		{"grpc", c.GRPC != nil, func() engine.Check { return grpccheck.New(*c.GRPC) }},
		{"ldap", c.LDAP != nil, func() engine.Check { return ldapcheck.New(*c.LDAP) }},
		{"kafka", c.Kafka != nil, func() engine.Check { return kafka.New(*c.Kafka) }},
		{"ingest", c.Ingest != nil, func() engine.Check { return ingest.New(*c.Ingest) }},
		{"s3", c.S3 != nil, func() engine.Check { return s3.New(*c.S3) }},
		{"smtp", c.SMTP != nil, func() engine.Check { return smtp.New(*c.SMTP) }},
		{"elasticsearch", c.Elasticsearch != nil, func() engine.Check { return elasticsearch.New(*c.Elasticsearch) }},
		{"mongodb", c.MongoDB != nil, func() engine.Check { return mongodb.New(*c.MongoDB) }},
		{"mysql", c.MySQL != nil, func() engine.Check { return mysql.New(*c.MySQL) }},
		{"etcd", c.Etcd != nil, func() engine.Check { return etcd.New(*c.Etcd) }},
		{"clickhouse", c.ClickHouse != nil, func() engine.Check { return clickhouse.New(*c.ClickHouse) }},
		{"vault", c.Vault != nil, func() engine.Check { return vault.New(*c.Vault) }},
		{"memcached", c.Memcached != nil, func() engine.Check { return memcached.New(*c.Memcached) }},
		{"cassandra", c.Cassandra != nil, func() engine.Check { return cassandra.New(*c.Cassandra) }},
	}
}

// OptionsFor returns base with any per-module override applied for module
// (CF-84): a non-zero override field wins, otherwise the base value is kept.
func OptionsFor(cfg *engine.Config, module string, base engine.Options) engine.Options {
	o, ok := cfg.ModuleOverrides[module]
	if !ok {
		return base
	}
	if o.TimeoutSeconds > 0 {
		base.Timeout = time.Duration(o.TimeoutSeconds) * time.Second
	}
	if o.Retries > 0 {
		base.Retries = o.Retries
	}
	if o.RetryBackoffMS > 0 {
		base.Backoff = time.Duration(o.RetryBackoffMS) * time.Millisecond
	}
	return base
}

// Jobs builds every configured module as an engine.Job, applying per-module
// option overrides on top of base. Registry order is preserved.
func Jobs(cfg *engine.Config, base engine.Options) []engine.Job {
	var jobs []engine.Job
	for _, s := range Modules(cfg) {
		if s.Configured {
			jobs = append(jobs, engine.Job{Check: s.Build(), Opts: OptionsFor(cfg, s.Name, base)})
		}
	}
	return jobs
}

// Configured builds every module present in the config, in registry order.
func Configured(cfg *engine.Config) []engine.Check {
	var checks []engine.Check
	for _, s := range Modules(cfg) {
		if s.Configured {
			checks = append(checks, s.Build())
		}
	}
	return checks
}

// Names returns the names of the modules present in the config.
func Names(cfg *engine.Config) []string {
	var names []string
	for _, s := range Modules(cfg) {
		if s.Configured {
			names = append(names, s.Name)
		}
	}
	return names
}

// All returns every known module name, whether configured or not.
func All(cfg *engine.Config) []string {
	specs := Modules(cfg)
	names := make([]string, len(specs))
	for i, s := range specs {
		names[i] = s.Name
	}
	return names
}
