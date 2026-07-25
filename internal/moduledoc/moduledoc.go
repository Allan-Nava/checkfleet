// Package moduledoc holds one-line explanations of what each check module does
// and its key thresholds. Shared by the CLI (`checkfleet explain`) and the
// desktop app so the descriptions live in a single place.
package moduledoc

// Docs maps a module name to a short description of what it checks.
var Docs = map[string]string{
	"certs":    "TLS certificate expiry per target/inventory host. WARN under `warn_days`, BAD under `crit_days` (or expired). Reads the leaf even on an untrusted chain.",
	"http":     "HTTP probe: BAD on unexpected status (`expect_status`), BAD if the body lacks `expect_body`, WARN over `max_latency_ms`, ERROR on network failure.",
	"nats":     "NATS JetStream cluster via `/varz`+`/jsz`: BAD if no meta-leader, WARN on lagging peers (`lag_warn`/`lag_crit`), ghost/missing peers (`expect_peers`), mixed versions.",
	"haproxy":  "HAProxy CSV stats: BAD on servers DOWN or a backend with no server, WARN on MAINT/DRAIN/NOLB and session saturation (`session_warn_pct`).",
	"stream":   "HLS/DASH manifest: BAD if unreachable/invalid, WARN on an incomplete bitrate ladder (`min_variants`) or a stale live-edge (`max_age_warn_seconds`/`max_age_crit_seconds`).",
	"patroni":  "Patroni cluster via REST: BAD if no leader, WARN on split-brain, replica lag (`lag_warn_bytes`/`lag_crit_bytes`) and timeline divergence.",
	"consul":   "Consul via HTTP API: BAD if no raft leader or below quorum, service checks `critical`->BAD/`warning`->WARN, missing `kv_keys`->BAD.",
	"postgres": "PostgreSQL via read-only SQL: wraparound age, connection saturation (`conn_warn_pct`), inactive replication slots retaining WAL, replica lag.",
	"dns":      "DNS resolution: records resolve, drift from `expect`, SOA-serial & answer consistency across resolvers, TTL under `min_ttl_seconds`.",
	"redis":    "Redis/Valkey `INFO`: reachability & role, memory vs `maxmemory` (`mem_warn_pct`), replication link/lag, RDB/AOF persistence.",
	"keycloak": "Keycloak health endpoint UP, and per-realm OIDC discovery (token endpoint present, issuer coherent with `/realms/<realm>`).",
	"tcp":      "Generic TCP reachability: connect (optionally TLS) + latency (`max_latency_ms`), optional `expect_banner` substring.",
	"tls":      "Deep TLS: chain validity vs trust store, leaf expiry (`warn_days`/`crit_days`), negotiated protocol (below TLS 1.2 -> WARN), hostname mismatch.",
	"ntp":      "NTP clock offset via SNTP: WARN/BAD over `offset_warn_ms`/`offset_crit_ms`, BAD if the server is unsynchronized (stratum 0 or >=16).",
	"rabbitmq": "RabbitMQ management API: node running + alarms, queue depth (`queue_warn_depth`/`queue_crit_depth`), backlog with no consumer.",
	"grpc":     "gRPC Health Checking Protocol over HTTP/2+TLS: SERVING->OK, NOT_SERVING->BAD, UNKNOWN->WARN.",
	"ldap":     "LDAP connect + bind (anonymous or creds from env), optional sanity search (at least `min_entries` under `base_dn`).",
	"kafka":    "Kafka cluster: controller present, brokers vs `expect_brokers`, under-replicated partitions, consumer-group lag (`lag_warn`/`lag_crit`).",
	"ingest":   "Streaming ingest reachability: RTMP handshake (TCP) or SRT induction handshake (UDP), with connect latency. Answers 'can the streamer publish?'.",
	"s3":       "S3/object storage: bucket reachable, optional sentinel object present and fresh (`max_age_warn_seconds`). AWS SigV4 signed (creds from env), path- or virtual-hosted style.",
	"smtp":     "SMTP relay: accepts connections, 220 greeting (optional `expect_banner`), EHLO ok, optional STARTTLS/implicit TLS and relay cert expiry (`warn_days`/`crit_days`). Never sends mail.",
}

// Doc returns the description for a module and whether it exists.
func Doc(name string) (string, bool) {
	d, ok := Docs[name]
	return d, ok
}
