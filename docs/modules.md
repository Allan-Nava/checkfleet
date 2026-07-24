---
title: Modules
nav_order: 5
---

Each module is a self-contained check that knows what "healthy" means for one
kind of target. Shipping today: `certs`, `http`, `nats`, `haproxy`, `stream`,
`patroni`, `consul`, `postgres`, `dns`, `redis`, `keycloak`, `tcp`, `tls`, `ntp`, `rabbitmq`. The
[backlog](https://github.com/Allan-Nava/checkfleet/blob/main/BACKLOG.md) tracks
what's next (`grpc`, `ldap`, `kafka`, `mongodb`, …).

## `certs`

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

## `http`

HTTP endpoint probes.

- Checks the response status against `expect_status` (mismatch → `BAD`).
- `WARN` when the response is slower than `max_latency_ms`.
- `BAD` when `expect_body` is set and its substring is missing.
- A network/transport error is `ERROR`.

See [Configuration → checks.http](configuration.md#checkshttp).

## `nats`

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

## `haproxy`

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

## `stream`

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

## `patroni`

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

## `consul`

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

## `postgres`

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

## `dns`

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

## `redis`

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

## `keycloak`

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

## `tcp`

Generic TCP reachability for anything that speaks TCP.

- Connects to each `address` (optionally over TLS) and measures the connect
  latency. A failed connect is `ERROR`.
- With `expect_banner`, the first bytes the server sends must contain the
  string, else `BAD` (e.g. `SSH-2.0` on port 22).
- With `max_latency_ms`, a slower connect is `WARN`.

See [Configuration → checks.tcp](configuration.md#checkstcp).

## `tls`

Deep TLS check — complements `certs` (which only reads leaf expiry).

- **Chain**: verifies the presented chain against the trust store (with the
  hostname); invalid (untrusted, hostname mismatch) → `BAD`.
- **Expiry**: leaf days-to-expiry → `OK`/`WARN`/`BAD` (`warn_days`/`crit_days`).
- **Protocol**: the negotiated version; below TLS 1.2 → `WARN` (connects
  permissively down to TLS 1.0 just to observe and flag it).
- Unreachable / handshake failure → `ERROR`.

Findings are labelled `target [chain]`, `target [expiry]`, `target [protocol]`.

See [Configuration → checks.tls](configuration.md#checkstls).

## `ntp`

NTP clock-offset check via a hand-rolled SNTP query (UDP, zero dependency).
Clock drift silently breaks TLS validation and JWT expiry.

- Estimated clock offset past `offset_warn_ms`/`offset_crit_ms` → `WARN`/`BAD`.
- An unsynchronized server (stratum 0 kiss-o'-death, or ≥16) → `BAD`.
- Unreachable / no reply → `ERROR`.

See [Configuration → checks.ntp](configuration.md#checksntp).

## `rabbitmq`

RabbitMQ health via the management HTTP API — read-only.

- **Reachability**: `/api/overview` (basic auth) → `OK` with version, else `ERROR`.
- **Nodes** (`/api/nodes`): not running, or a memory / disk-free alarm → `BAD`.
- **Queues** (`/api/queues`): depth past `queue_warn_depth`/`queue_crit_depth` →
  `WARN`/`BAD`; messages present with zero consumers → `WARN` (stuck backlog).

Any management node answers cluster-wide, so one endpoint is enough. Findings
are labelled `node/<name>` and `queue/<vhost>/<name>`. The password is read from
`password_env`, never stored in config.

See [Configuration → checks.rabbitmq](configuration.md#checksrabbitmq).

## Ansible inventory as a target source

The `certs`, `nats`, `haproxy`, `patroni`, `consul`, `redis` and `tls` modules can read
a standard Ansible **INI** inventory (a file or a directory of files):

- host lines and their `ansible_host=` value are used;
- `:vars` and `:children` sections are ignored;
- hosts are de-duplicated.

Every discovered host becomes a target on the module's `port` (443 for `certs`,
8222 for `nats`, 8404 for `haproxy`, 8008 for `patroni`, 8500 for `consul`,
6379 for `redis`, 443 for `tls`).
