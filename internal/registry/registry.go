// Package registry is the single source of truth for which check modules exist
// and how each is built from the typed config. Both the CLI (cmd/checkfleet)
// and the desktop app (desktop/) wire their checks through here so the module
// list lives in exactly one place — adding a module means editing Modules only.
package registry

import (
	"github.com/Allan-Nava/checkfleet/internal/checks/certs"
	"github.com/Allan-Nava/checkfleet/internal/checks/consul"
	"github.com/Allan-Nava/checkfleet/internal/checks/dns"
	"github.com/Allan-Nava/checkfleet/internal/checks/grpccheck"
	"github.com/Allan-Nava/checkfleet/internal/checks/haproxy"
	"github.com/Allan-Nava/checkfleet/internal/checks/httpcheck"
	"github.com/Allan-Nava/checkfleet/internal/checks/kafka"
	"github.com/Allan-Nava/checkfleet/internal/checks/keycloak"
	"github.com/Allan-Nava/checkfleet/internal/checks/ldapcheck"
	"github.com/Allan-Nava/checkfleet/internal/checks/nats"
	"github.com/Allan-Nava/checkfleet/internal/checks/ntp"
	"github.com/Allan-Nava/checkfleet/internal/checks/patroni"
	"github.com/Allan-Nava/checkfleet/internal/checks/postgres"
	"github.com/Allan-Nava/checkfleet/internal/checks/rabbitmq"
	"github.com/Allan-Nava/checkfleet/internal/checks/redis"
	"github.com/Allan-Nava/checkfleet/internal/checks/stream"
	"github.com/Allan-Nava/checkfleet/internal/checks/tcp"
	"github.com/Allan-Nava/checkfleet/internal/checks/tlscheck"
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
	}
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
