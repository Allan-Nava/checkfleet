---
title: Modules
nav_order: 5
description: >-
  Every checkfleet module and what it actually verifies — TLS certificates,
  HTTP, DNS, NATS, Kafka, PostgreSQL, MySQL, Redis, MongoDB, Consul, Vault,
  HAProxy, S3, SMTP and more.
---

Each module is a self-contained check that knows what "healthy" means for one
kind of target. **{{ site.data.modules | size }} modules ship today**; the
[backlog](https://github.com/Allan-Nava/checkfleet/blob/main/BACKLOG.md) tracks
what's next.

| Module | Checks | What it tells you |
|---|---|---|
{% for m in site.data.modules -%}
| [`{{ m.name }}`](#{{ m.name }}) | {{ m.title }} | {{ m.summary }} |
{% endfor %}

## `certs` — TLS certificates {#certs}

TLS certificate expiry across a fleet.

- Dials each target with SNI and reads the leaf certificate's `NotAfter`.
- Reports `OK`, `WARN` (expires within `warn_days`), or `BAD` (within
  `crit_days` or already expired).
- A dial/handshake failure is `ERROR` (couldn't measure), not `BAD`.
- Targets come from the explicit `targets` list **and/or** every host of an
  Ansible INI inventory (`ansible_inventory`). Probes run concurrently.

> The dial uses `InsecureSkipVerify` **on purpose**: we want the expiry date even
> when the chain doesn't validate locally. It is an expiry reader, not a chain
> validator.

See [Configuration → checks.certs](configuration.md#checkscerts).

## `http` — HTTP endpoints {#http}

HTTP endpoint probes.

- Checks the response status against `expect_status` (mismatch → `BAD`).
- `WARN` when the response is slower than `max_latency_ms`.
- `BAD` when `expect_body` is set and its substring is missing.
- A network/transport error is `ERROR`.

See [Configuration → checks.http](configuration.md#checkshttp).

## `nats` — NATS JetStream {#nats}

Preflight/health of a NATS JetStream cluster, read from each node's HTTP
monitoring port (`/varz` and `/jsz?meta=1`) — the read-only endpoints only, it
never mutates the cluster. It encodes the operational signals from the ops
runbook:

- **Reachability + version** per node (`OK` with `server_name`, version, conns,
  uptime; `ERROR` if the monitoring port doesn't answer).
- **Mixed versions** across the cluster → `WARN` (e.g. mid-upgrade skew).
- **Meta-leader**: `BAD` if no meta-leader is elected (quorum lost), `WARN` if
  the elected leader disagrees across nodes, or if it isn't the
  `expect_meta_leader` you configured.
- **Peer health** (from the meta raft group): `BAD` if a peer is `OFFLINE`,
  `WARN` if `not current`.
- **Peer lag**: `WARN`/`BAD` when a peer's raft lag crosses `lag_warn`/`lag_crit`.
- **Ghost / missing peers**: with `expect_peers` set, an unexpected member is a
  `WARN` (ghost), an expected member absent from the cluster is `BAD`.

See [Configuration → checks.nats](configuration.md#checksnats).

## `haproxy` — HAProxy {#haproxy}

Backend/server health from the HAProxy **CSV stats export** over HTTP (the
`;csv` stats endpoint) — read-only, it never mutates HAProxy.

- Per server: `UP` → `OK`, `DOWN` → `BAD`, `MAINT`/`DRAIN`/`NOLB` → `WARN`.
- Per backend (the `BACKEND` aggregate row): `DOWN` → `BAD` (no server
  available).
- Optional session saturation: with `session_warn_pct`, a server/backend at or
  above that percent of its session limit (`scur/slim`) is `WARN`.
- An unreachable stats page is `ERROR`. Frontends are skipped to keep the output
  signal-dense.

Findings are labelled `backend/server` (e.g. `web/web2`). Optional HTTP basic
auth is supported, with the password read from an env var — never stored in the
config.

See [Configuration → checks.haproxy](configuration.md#checkshaproxy).

## `stream` — HLS / DASH streams {#stream}

HLS and DASH stream health, read from the manifest — it fetches only manifests,
never media segments.

- **Reachability / validity**: an unreachable manifest is `ERROR`; a manifest
  that doesn't parse (bad `.m3u8` / `.mpd`) is `BAD`.
- **Ladder completeness**: with `min_variants`, a master playlist (HLS) or MPD
  (DASH) with fewer renditions is `WARN`, with none is `BAD`.
- **Live-edge freshness** (when `live: true`): the age of the live edge — from
  HLS `#EXT-X-PROGRAM-DATE-TIME` advanced by segment durations, or DASH
  `publishTime` — is `WARN`/`BAD` past `max_age_warn_seconds`/`max_age_crit_seconds`.
  If `live` is set but the manifest is VOD (HLS `#EXT-X-ENDLIST`, or a static
  MPD), that's a `WARN`.
- Freshness needs a timestamp in the manifest: an HLS live playlist without
  `#EXT-X-PROGRAM-DATE-TIME` reports `WARN` ("not measurable") rather than a
  false OK.

For an HLS **master** playlist with `live: true`, the check fetches the
highest-bandwidth variant to measure its live edge. Findings are labelled
`name`, `name [ladder]`, `name [live-edge]`. Format (HLS vs DASH) is detected
from the `.mpd` extension, the `dash+xml` content-type, or an `<MPD` root.

See [Configuration → checks.stream](configuration.md#checksstream).

## `patroni` — Patroni PostgreSQL {#patroni}

Health of a Patroni-managed PostgreSQL cluster from the Patroni **REST API**
(`/cluster`) — read-only, it never touches PostgreSQL itself.

- **Leader**: exactly one expected. Zero leaders → `BAD` (failover in progress
  or lost quorum); more than one → `WARN` (split-brain).
- **Replica state**: `running`/`streaming` → `OK`; `stopped`/`start failed`/
  `crashed`/`restarting` → `BAD`; anything else → `WARN`.
- **Replica lag**: `WARN`/`BAD` past `lag_warn_bytes`/`lag_crit_bytes`. A lag
  reported as `unknown` is `OK` with a note (not a false alarm).
- **Timeline**: a replica on a different timeline than the leader → `WARN`.

Any Patroni node proxies the same cluster view, so listing one endpoint is
enough; extra endpoints add redundancy (an unreachable one is `ERROR`). The
cluster-level leader finding is labelled with the cluster scope; per-node
findings are labelled by member name.

See [Configuration → checks.patroni](configuration.md#checkspatroni).

## `consul` — HashiCorp Consul {#consul}

Consul cluster health via the HTTP API — read-only.

- **Raft leader**: `/v1/status/leader` empty → `BAD` (no leader elected).
- **Raft peers**: `/v1/status/peers` count vs `expect_peers` — below quorum
  (majority) → `BAD`, below expected but with quorum → `WARN`.
- **Service/node health**: every check in `/v1/health/state/critical` → `BAD`,
  every one in `.../warning` → `WARN`, labelled `service@node`.
- **KV keys**: each key in `kv_keys` missing (`/v1/kv/<key>` → 404) → `BAD`.

Any agent answers cluster-wide, so one endpoint is enough; extras add
redundancy (an unreachable one is `ERROR`). An ACL token can be supplied via
`token_env` (read from the environment, never stored in config).

See [Configuration → checks.consul](configuration.md#checksconsul).

## `postgres` — PostgreSQL {#postgres}

PostgreSQL health via **read-only SQL** (using the `pgx` driver — the module's
own dependency). It never runs DDL or writes.

- **Reachability**: a failed connect or query is `ERROR`; otherwise `OK` with
  the role (`primary`/`replica`, from `pg_is_in_recovery()`).
- **Transaction wraparound**: `max(age(datfrozenxid))` past
  `wraparound_warn_age`/`wraparound_crit_age` → `WARN`/`BAD` (wraparound looms
  near ~2.1e9).
- **Connection saturation**: `WARN` when active connections reach
  `conn_warn_pct`% of `max_connections`.
- **Inactive replication slots**: an inactive slot is `WARN`; if it retains WAL
  past `slot_warn_bytes`/`slot_crit_bytes` → `WARN`/`BAD` (disk-fill risk).
- **Replica lag** (primary only, from `pg_stat_replication`): `WARN`/`BAD` past
  `lag_warn_bytes`/`lag_crit_bytes`.

Findings are labelled `name`, `name [wraparound]`, `name [connections]`,
`name [slot:<name>]`, `name [repl:<client>]`. The password is read from the
target's `password_env` — never stored in the config.

See [Configuration → checks.postgres](configuration.md#checkspostgres).

## `dns` — DNS {#dns}

DNS resolution health, using a small in-tree DNS client (no third-party
dependency) so it can query specific resolvers and read TTLs and SOA serials.

- **Resolution**: a name that no resolver answers is `ERROR`; a name that
  resolves to no record of the requested type is `BAD`.
- **Drift**: with `expect`, an answer set different from the expected values is
  `BAD` (for `SOA` the serial is compared).
- **Consistency across resolvers**: when resolvers return different answers —
  including divergent SOA serials (a propagation lag) — that's `WARN`; so is a
  resolver that fails to answer while others succeed.
- **TTL**: with `min_ttl_seconds`, an answer TTL below the threshold is `WARN`.

Supported record types: `A`, `AAAA`, `CNAME`, `NS`, `TXT`, `SOA`. Resolvers
default to the system `/etc/resolv.conf` when none are configured. Findings are
labelled `name/TYPE`, `name/TYPE [consistency]`, `name/TYPE [ttl]`.

See [Configuration → checks.dns](configuration.md#checksdns).

## `redis` — Redis / Valkey {#redis}

Redis / Valkey health via a minimal in-tree RESP client (no third-party
dependency) reading `INFO` — read-only commands only.

- **Reachability**: a failed connect/`PING`/`INFO` is `ERROR`; otherwise `OK`
  with version and role. `WARN` while the dataset is still `loading`.
- **Memory**: with `mem_warn_pct` and a configured `maxmemory`, `used_memory` at
  or above that percent is `WARN`.
- **Replication** (replicas): `master_link_status` not `up` → `BAD`; the
  master/replica offset lag past `lag_warn_bytes`/`lag_crit_bytes` → `WARN`/`BAD`.
- **Persistence**: a failed last RDB bgsave, or AOF last write when AOF is
  enabled, → `WARN`.

Findings are labelled `target`, `target [memory]`, `target [replication]`,
`target [persistence]`. TLS (`rediss`) and ACL auth are supported; the password
is read from `password_env`, never stored in config.

See [Configuration → checks.redis](configuration.md#checksredis).

## `keycloak` — Keycloak {#keycloak}

Keycloak health via HTTP/JSON — read-only, no admin credentials.

- **Health** (when `health_url` is set): the endpoint (e.g. `/health/ready`,
  often on the management port) must report `status: UP` → `OK`; `DOWN` → `BAD`;
  unreachable → `ERROR`.
- **Per realm**: the OIDC discovery document
  (`/realms/<realm>/.well-known/openid-configuration`) must return `200` with a
  `token_endpoint` → `OK`. A `404`/invalid document → `BAD` (realm missing); an
  `issuer` that doesn't end with `/realms/<realm>` → `WARN` (usually a
  proxy/frontend-URL misconfiguration); unreachable → `ERROR`.

Findings are labelled `health` and `realm/<name>`.

See [Configuration → checks.keycloak](configuration.md#checkskeycloak).

## `tcp` — TCP services {#tcp}

Generic TCP reachability for anything that speaks TCP.

- Connects to each `address` (optionally over TLS) and measures the connect
  latency. A failed connect is `ERROR`.
- With `expect_banner`, the first bytes the server sends must contain the
  string, else `BAD` (e.g. `SSH-2.0` on port 22).
- With `max_latency_ms`, a slower connect is `WARN`.

See [Configuration → checks.tcp](configuration.md#checkstcp).

## `tls` — TLS handshakes {#tls}

Deep TLS check — complements `certs` (which only reads leaf expiry).

- **Chain**: verifies the presented chain against the trust store (with the
  hostname); invalid (untrusted, hostname mismatch) → `BAD`.
- **Expiry**: leaf days-to-expiry → `OK`/`WARN`/`BAD` (`warn_days`/`crit_days`).
- **Protocol**: the negotiated version; below TLS 1.2 → `WARN` (connects
  permissively down to TLS 1.0 just to observe and flag it).
- Unreachable / handshake failure → `ERROR`.

Findings are labelled `target [chain]`, `target [expiry]`, `target [protocol]`.

See [Configuration → checks.tls](configuration.md#checkstls).

## `ntp` — NTP / clock drift {#ntp}

NTP clock-offset check via a hand-rolled SNTP query (UDP, zero dependency).
Clock drift silently breaks TLS validation and JWT expiry.

- Estimated clock offset past `offset_warn_ms`/`offset_crit_ms` → `WARN`/`BAD`.
- An unsynchronized server (stratum 0 kiss-o'-death, or ≥16) → `BAD`.
- Unreachable / no reply → `ERROR`.

See [Configuration → checks.ntp](configuration.md#checksntp).

## `rabbitmq` — RabbitMQ {#rabbitmq}

RabbitMQ health via the management HTTP API — read-only.

- **Reachability**: `/api/overview` (basic auth) → `OK` with version, else `ERROR`.
- **Nodes** (`/api/nodes`): not running, or a memory / disk-free alarm → `BAD`.
- **Queues** (`/api/queues`): depth past `queue_warn_depth`/`queue_crit_depth` →
  `WARN`/`BAD`; messages present with zero consumers → `WARN` (stuck backlog).

Any management node answers cluster-wide, so one endpoint is enough. Findings
are labelled `node/<name>` and `queue/<vhost>/<name>`. The password is read from
`password_env`, never stored in config.

See [Configuration → checks.rabbitmq](configuration.md#checksrabbitmq).

## `grpc` — gRPC services {#grpc}

gRPC Health Checking Protocol (`grpc.health.v1.Health/Check`) over **HTTP/2 +
TLS**, with the protobuf messages encoded by hand — no gRPC library dependency.
(Plaintext h2c isn't supported; TLS endpoints only.)

- `SERVING` → `OK`; `NOT_SERVING` / `SERVICE_UNKNOWN` → `BAD`; `UNKNOWN` → `WARN`.
- `grpc-status 12` (UNIMPLEMENTED, no health service) → `WARN`; `5` (NOT_FOUND,
  unknown service) → `BAD`.
- Connection/handshake failure → `ERROR`.

Set `service` to check a specific gRPC service, or leave empty for whole-server
health. `insecure_skip_verify` for internal self-signed endpoints.

See [Configuration → checks.grpc](configuration.md#checksgrpc).

## `ldap` — LDAP directories {#ldap}

LDAP directory health via bind + an optional sanity search (uses `go-ldap`).

- Connect failure → `ERROR`; failed bind (bad credentials) → `BAD`.
- With `base_dn`, a search returning fewer than `min_entries` (default 1) or an
  error → `BAD`.
- `ldaps://` and `start_tls` supported; `insecure_skip_verify` for internal
  self-signed. Bind is anonymous when `bind_dn` is empty; the password comes
  from `password_env`, never config.

See [Configuration → checks.ldap](configuration.md#checksldap).

## `kafka` — Apache Kafka {#kafka}

Kafka cluster health via `franz-go`/`kadm` (admin metadata only).

- Metadata unreachable → `ERROR`; no controller → `BAD`; fewer than
  `expect_brokers` → `WARN`.
- Any under-replicated partition (ISR < replicas) → `BAD`.
- Per consumer group in `groups`: total lag past `lag_warn`/`lag_crit` →
  `WARN`/`BAD`.

Findings: `cluster`, `partitions`, `group/<name>`. Optional TLS and SASL
(plain/scram); the SASL password comes from `sasl_password_env`, never config.
The Kafka I/O is behind an interface, so the finding logic is unit-tested with
a fake — no real broker in tests.

See [Configuration → checks.kafka](configuration.md#checkskafka).

## `ingest` — RTMP / SRT ingest {#ingest}

Answers "can the streamer publish?" by speaking just enough of each protocol to
prove a real server is listening, not just an open port:

- **RTMP** (`protocol: rtmp`, default): the RTMP simple handshake over TCP
  (C0/C1 → S0/S1/S2, version checked).
- **SRT** (`protocol: srt`): the SRT **induction** handshake over UDP (a
  best-effort reachability probe — it confirms an SRT listener answers).

`OK` on a completed handshake, `WARN` over `max_latency_ms`, `ERROR` if the
connection or handshake fails, `BAD` on an unknown protocol. Zero dependencies;
tested against in-test fake RTMP/SRT servers.

## `s3` — S3 object storage {#s3}

Checks an S3-compatible bucket (AWS S3, MinIO, Ceph):

- **Bucket reachable** — `OK` on 200, `BAD` on 404 (missing) / 403 (denied).
- **Sentinel object** (optional `object`) — `BAD` if missing, `WARN` if older
  than `max_age_warn_seconds` (a stale drop/backup), else `OK`.

Requests are signed with **AWS Signature V4 written by hand** (zero deps, no AWS
SDK); credentials come from `access_key_env`/`secret_key_env` (env only), or it
falls back to anonymous for public buckets. `path_style: true` for MinIO/Ceph.
Tested against an in-test fake S3 (`httptest`).

## `smtp` — SMTP relays {#smtp}

Verifies an SMTP relay is healthy **without ever sending mail**:

- **Connection & greeting** — `ERROR` if it can't connect, `BAD` on a non-`220`
  greeting or a greeting missing the optional `expect_banner` substring.
- **EHLO** — `BAD` if `EHLO` is rejected.
- **STARTTLS** (`starttls: true`) — `BAD` if not advertised or the upgrade fails.
- **Implicit TLS** (`tls: true`, e.g. port 465) — wraps the connection at once.
- **Relay certificate** — when TLS is negotiated, reads the leaf and reports
  `WARN` under `warn_days`, `BAD` under `crit_days` (or expired).
- **Latency** — `WARN` over `max_latency_ms`.

Default port is `25`, or `465` when `tls: true`. Zero-dep (stdlib `net`/`crypto/tls`);
tested against in-test fake relays (plain, STARTTLS, implicit TLS).

```yaml
  smtp:
    warn_days: 30
    crit_days: 7
    targets:
      - {name: relay, address: mail.example.com:25, starttls: true}
      - {name: smtps, address: mail.example.com:465, tls: true}
```

## `elasticsearch` — Elasticsearch / OpenSearch {#elasticsearch}

Checks an Elasticsearch or OpenSearch cluster over its HTTP API (any node
answers cluster-wide):

- **Cluster health** (`/_cluster/health`) — `green` → `OK`, `yellow` → `WARN`,
  `red` → `BAD`; the message carries node count, unassigned shards and active
  shard percentage.
- **Expected nodes** (`expect_nodes`) — `BAD` if the cluster reports fewer nodes
  than expected (a shrunk cluster, even when still green).
- **Disk watermark** (`/_cat/allocation`) — per node, `WARN` over
  `disk_warn_pct` (default 85, ES low watermark), `BAD` over `disk_crit_pct`
  (default 90, high watermark).
- `ERROR` if the API is unreachable.

Credentials come from env — `username` + `password_env` (basic auth) or
`api_key_env` (API key) — never inline; `insecure_skip_verify` for self-signed
clusters. Zero-dep (HTTP/JSON); tested against an in-test fake cluster.

```yaml
  elasticsearch:
    disk_warn_pct: 85
    disk_crit_pct: 90
    targets:
      - {name: logs, url: https://es.example.com:9200, username: elastic, password_env: ES_PASSWORD, expect_nodes: 3}
```

## `mongodb` — MongoDB {#mongodb}

Read-only health check for MongoDB via the **official driver** (any member
answers cluster-wide):

- **Replica-set status** (`replSetGetStatus`) — `BAD` if there is no healthy
  `PRIMARY`, `BAD` for any member with health 0 (unreachable), and per-secondary
  replication lag: `WARN` over `lag_warn_seconds`, `BAD` over `lag_crit_seconds`.
  A standalone node (not a replica set) is reported as reachable and the
  replica-set checks are skipped.
- **Connections** (`serverStatus`) — `WARN` when in-use connections exceed
  `conn_warn_pct` of the total available.
- `ERROR` if the deployment is unreachable or the status query fails.

Credentials come from env (`username` + `password_env`, never in the URI/config);
`auth_source` defaults to `admin`. Uses `go.mongodb.org/mongo-driver/v2` (a
motivated exception to the zero-dep rule, like `pgx` for Postgres). The finding
logic is unit-tested with a fake collector — no real database in tests.

```yaml
  mongodb:
    lag_warn_seconds: 10
    lag_crit_seconds: 60
    targets:
      - {name: rs0, uri: "mongodb://m1:27017,m2:27017/?replicaSet=rs0", username: monitor, password_env: MONGO_PASSWORD}
```

## `mysql` — MySQL / MariaDB {#mysql}

Read-only health check via the standard `go-sql-driver/mysql` (a motivated
exception to the zero-dep rule, like `pgx` for Postgres):

- **Reachable & role** — `OK` with the server version; reports whether the server
  is read-only.
- **Connections** — `WARN` when `Threads_connected` exceeds `conn_warn_pct` of
  `max_connections`.
- **Replication** (only on a replica) — `BAD` if the IO or SQL thread is not
  running or the replica is not replicating (`Seconds_Behind` is NULL); otherwise
  lag `WARN`/`BAD` over `lag_warn_seconds`/`lag_crit_seconds`. Works with both the
  modern `SHOW REPLICA STATUS` and the legacy `SHOW SLAVE STATUS` column names.
- `ERROR` if the server is unreachable or a query fails.

Put the password in the DSN via `${ENV}` interpolation — it is resolved from the
environment at config load, never stored inline. The finding logic is unit-tested
with a fake collector; no real database in tests.

```yaml
  mysql:
    lag_warn_seconds: 10
    lag_crit_seconds: 60
    targets:
      - {name: primary, dsn: "monitor:${MYSQL_PASSWORD}@tcp(db-01:3306)/"}
```

## `etcd` — etcd {#etcd}

Checks an etcd v3 cluster over its **HTTP JSON gateway** — no `clientv3`
dependency:

- **Health** (`/health`) — `ERROR` if unreachable, `BAD` if the endpoint reports
  unhealthy.
- **Leader** (`/v3/maintenance/status`) — `BAD` if there is no leader (the
  cluster has lost quorum); otherwise reports the etcd version.
- **Members** (`/v3/cluster/member/list`) — `BAD` if fewer members than
  `expect_members` (quorum risk), else the count is shown in the healthy message.

Optional token auth (`username` + `password_env`, resolved from the environment)
and `insecure_skip_verify` for self-signed clusters. Zero-dep (HTTP/JSON); tested
against an in-test fake gateway.

```yaml
  etcd:
    expect_members: 3
    targets:
      - {name: etcd-01, url: https://etcd-01:2379, insecure_skip_verify: true}
```

## `clickhouse` — ClickHouse {#clickhouse}

Checks a ClickHouse server over its HTTP interface (zero-dep):

- **Reachability** — `/ping` must return `Ok.` (`ERROR` if unreachable, `BAD`
  otherwise) and `SELECT version()` must answer (`ERROR` on failure); the healthy
  message carries the server version.
- **Replicated tables** (`system.replicas`) — per table, `BAD` if the replica is
  read-only (usually a lost ZooKeeper/Keeper session), else replication delay
  `WARN`/`BAD` over `delay_warn_seconds`/`delay_crit_seconds`. Healthy tables
  produce no finding; a server with no replicated tables adds nothing.

Credentials come from env (`username` + `password_env`, sent as HTTP basic auth),
never inline; `insecure_skip_verify` for self-signed HTTPS. Tested against an
in-test fake HTTP server.

```yaml
  clickhouse:
    delay_warn_seconds: 30
    delay_crit_seconds: 300
    targets:
      - {name: ch-01, url: http://ch-01:8123, username: monitor, password_env: CLICKHOUSE_PASSWORD}
```

## `vault` — HashiCorp Vault {#vault}

Checks a Vault node over its HTTP API (zero-dep):

- **Seal status** (`/v1/sys/seal-status`) — `ERROR` if unreachable, `BAD` if the
  node is sealed (with unseal progress `n/threshold`) or not initialized.
- **Role** (`/v1/sys/health`) — reports `active` or `standby` (both `OK` — standby
  is normal in an HA cluster) with the Vault version.

Both endpoints are unauthenticated; an optional `token_env` sends `X-Vault-Token`
for setups that restrict them. `insecure_skip_verify` for self-signed HTTPS.
Tested against an in-test fake Vault.

```yaml
  vault:
    targets:
      - {name: vault-01, url: https://vault-01:8200}
```

## `memcached` — memcached {#memcached}

Checks memcached over its text protocol (zero-dep):

- **Reachability** — connects and runs `STATS`; `ERROR` if it can't.
- **Memory** — `WARN` when `bytes` exceeds `mem_warn_pct` of `limit_maxbytes`
  (default 90); otherwise `OK` with version, memory percentage and connection
  count. The percentage is also the target's numeric metric (`%`).
- **Evictions** — a `[evictions]` finding carrying the counter as a metric.
  memcached only exposes a **total since startup**, not a rate, so there is no
  useful default threshold: the count is published so the history can chart it
  (a rising line is the real signal), and it only `WARN`s when you set an
  explicit `evictions_warn`. Omitted when the server doesn't report the stat.

Targets are `host[:port]` (default port `11211`). Tested against an in-test fake
memcached.

```yaml
  memcached:
    mem_warn_pct: 90
    evictions_warn: 0   # 0 = report only, no threshold
    targets: [cache-01, cache-02:11212]
```

## `cassandra` — Cassandra / ScyllaDB {#cassandra}

Reachability check that speaks the **CQL native protocol** handshake directly —
no driver, no authentication:

- Connects and performs `OPTIONS` → `SUPPORTED` (negotiating the CQL version),
  then `STARTUP`. A `READY` or `AUTHENTICATE` reply means the node accepts CQL
  connections → `OK` (with `(auth required)` noted when authentication is on).
- `WARN` when the handshake is slower than `max_latency_ms`. Handshake latency
  is also the node's numeric metric (`ms`).
- `BAD` on a protocol `ERROR` reply, `ERROR` if the node is unreachable.
- **Cluster state** — a `cluster` finding rolls the nodes up: how many accept
  CQL out of those configured, as a metric (`nodes`). `BAD` below
  `expect_nodes` (0 = expect them all), `WARN` when the expectation is met but
  a configured node is still down. A slow node (`WARN`) still counts as up —
  it completed the handshake. Skipped for a single target with no
  `expect_nodes`, where it would only repeat the node's own finding.

  This is derived from checkfleet's own probes, not from `system.peers`:
  reading the cluster's view of its membership needs a `QUERY` on an
  authenticated session, and this module speaks the handshake only. The
  trade-off is deliberate — it says nothing about nodes absent from your
  config, but it keeps working on clusters with authentication enabled, with
  no driver and no credentials. Same shape as `etcd`'s `expect_members`.

Targets are `host[:port]` (default CQL port `9042`). Zero-dep; tested against an
in-test fake CQL server.

```yaml
  cassandra:
    expect_nodes: 3   # 0 = all configured nodes must accept CQL
    targets:
      - {name: cass-01, address: cass-01:9042, max_latency_ms: 500}
```

## Ansible inventory as a target source

The `certs`, `nats`, `haproxy`, `patroni`, `consul`, `redis` and `tls` modules can read
a standard Ansible **INI** inventory (a file or a directory of files):

- host lines and their `ansible_host=` value are used;
- `:vars` and `:children` sections are ignored;
- hosts are de-duplicated.

Every discovered host becomes a target on the module's `port` (443 for `certs`,
8222 for `nats`, 8404 for `haproxy`, 8008 for `patroni`, 8500 for `consul`,
6379 for `redis`, 443 for `tls`).
