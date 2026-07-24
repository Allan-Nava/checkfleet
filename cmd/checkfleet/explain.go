package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/registry"
)

// moduleDocs explains, per module, what it checks and the key thresholds. Kept
// in sync with the registry by a test (every module must have an entry).
var moduleDocs = map[string]string{
	"certs":    "TLS certificate expiry per target/inventory host. WARN under `warn_days`, BAD under `crit_days` (or expired). Reads the leaf even on an untrusted chain.",
	"http":     "HTTP probe: BAD on unexpected status (`expect_status`), BAD if the body lacks `expect_body`, WARN over `max_latency_ms`, ERROR on network failure.",
	"nats":     "NATS JetStream cluster via `/varz`+`/jsz`: BAD if no meta-leader, WARN on lagging peers (`lag_warn`/`lag_crit`), ghost/missing peers (`expect_peers`), mixed versions.",
	"haproxy":  "HAProxy CSV stats: BAD on servers DOWN or a backend with no server, WARN on MAINT/DRAIN/NOLB and session saturation (`session_warn_pct`).",
	"stream":   "HLS/DASH manifest: BAD if unreachable/invalid, WARN on an incomplete bitrate ladder (`min_variants`) or a stale live-edge (`max_age_warn_seconds`/`max_age_crit_seconds`).",
	"patroni":  "Patroni cluster via REST: BAD if no leader, WARN on split-brain, replica lag (`lag_warn_bytes`/`lag_crit_bytes`) and timeline divergence.",
	"consul":   "Consul via HTTP API: BAD if no raft leader or below quorum, service checks `critical`→BAD/`warning`→WARN, missing `kv_keys`→BAD.",
	"postgres": "PostgreSQL via read-only SQL: wraparound age, connection saturation (`conn_warn_pct`), inactive replication slots retaining WAL, replica lag.",
	"dns":      "DNS resolution: records resolve, drift from `expect`, SOA-serial & answer consistency across resolvers, TTL under `min_ttl_seconds`.",
	"redis":    "Redis/Valkey `INFO`: reachability & role, memory vs `maxmemory` (`mem_warn_pct`), replication link/lag, RDB/AOF persistence.",
	"keycloak": "Keycloak health endpoint UP, and per-realm OIDC discovery (token endpoint present, issuer coherent with `/realms/<realm>`).",
	"tcp":      "Generic TCP reachability: connect (optionally TLS) + latency (`max_latency_ms`), optional `expect_banner` substring.",
	"tls":      "Deep TLS: chain validity vs trust store, leaf expiry (`warn_days`/`crit_days`), negotiated protocol (< TLS 1.2 → WARN), hostname mismatch.",
	"ntp":      "NTP clock offset via SNTP: WARN/BAD over `offset_warn_ms`/`offset_crit_ms`, BAD if the server is unsynchronized (stratum 0/≥16).",
	"rabbitmq": "RabbitMQ management API: node running + alarms, queue depth (`queue_warn_depth`/`queue_crit_depth`), backlog with no consumer.",
	"grpc":     "gRPC Health Checking Protocol over HTTP/2+TLS: SERVING→OK, NOT_SERVING→BAD, UNKNOWN→WARN.",
	"ldap":     "LDAP connect + bind (anonymous or creds from env), optional sanity search (≥ `min_entries` under `base_dn`).",
	"kafka":    "Kafka cluster: controller present, brokers vs `expect_brokers`, under-replicated partitions, consumer-group lag (`lag_warn`/`lag_crit`).",
}

// runExplain prints what a module checks and its thresholds, or lists modules.
//
//	checkfleet explain [module]
func runExplain(args []string) error {
	all := registry.All(&engine.Config{})
	if len(args) == 0 {
		fmt.Println("modules (checkfleet explain <module>):")
		sorted := append([]string(nil), all...)
		sort.Strings(sorted)
		for _, m := range sorted {
			fmt.Printf("  %-9s %s\n", m, firstSentence(moduleDocs[m]))
		}
		return nil
	}
	m := args[0]
	doc, ok := moduleDocs[m]
	if !ok {
		return fmt.Errorf("unknown module %q (run: checkfleet explain)", m)
	}
	fmt.Printf("%s — %s\n", m, doc)
	return nil
}

func firstSentence(s string) string {
	if i := strings.IndexByte(s, '.'); i >= 0 {
		return s[:i+1]
	}
	return s
}

// moduleNames returns every known module name (for completion/help).
func moduleNames() []string {
	names := registry.All(&engine.Config{})
	sort.Strings(names)
	return names
}
