---
title: Configuration
nav_order: 3
---

checkfleet reads a single YAML file (default `checkfleet.yml`, override with
`--config`). A [`checkfleet.example.yml`](https://github.com/Allan-Nava/checkfleet/blob/main/checkfleet.example.yml)
ships with the repo — copy it and adapt.

```yaml
# checkfleet.yml
timeout_seconds: 30          # global deadline for a whole run (default 30)

checks:
  certs:
    warn_days: 30            # WARN when the cert expires within N days (default 30)
    crit_days: 7             # BAD  when it expires within N days (default 7)
    port: 443                # default port for targets/inventory hosts (default 443)
    targets:
      - example.com          # uses the default port
      - internal.example:8443
    ansible_inventory: /path/to/inventory   # optional: every host → target on `port`

  http:
    targets:
      - url: https://example.com/
        expect_status: 200   # expected HTTP status (default 200)
        max_latency_ms: 2000 # WARN if the response is slower than this
        expect_body: "ok"    # BAD if this substring is missing from the body
```

## Top-level keys

| Key | Type | Default | Meaning |
|---|---|---|---|
| `timeout_seconds` | int | `30` | Per-check (and per-attempt) deadline. |
| `retries` | int | `0` | Retry a check that produced an ERROR finding (transient network/handshake), up to this many times. |
| `retry_backoff_ms` | int | `500` (when `retries`>0) | Base backoff between attempts; doubles each retry. |
| `checks` | map | — | One entry per module. A module runs only if its key is present. |

A module that is **not** present in `checks` is skipped by `check all`, and
`check <name>` for it fails with `modulo "<name>" non configurato`.

## `checks.certs`

TLS certificate expiry. See [Modules → certs](modules.md#certs).

| Key | Type | Default | Meaning |
|---|---|---|---|
| `warn_days` | int | `30` | Days-to-expiry threshold for WARN. |
| `crit_days` | int | `7` | Days-to-expiry threshold for BAD. |
| `port` | int | `443` | Default port for targets and inventory hosts without an explicit `:port`. |
| `targets` | list | — | `host` or `host:port` entries. |
| `ansible_inventory` | string | — | Path to an Ansible INI inventory (file or directory). Every host becomes a target on `port`. |

Targets and inventory hosts are merged and de-duplicated. At least one of
`targets` / `ansible_inventory` should be set.

## `checks.http`

HTTP probes. See [Modules → http](modules.md#http).

`checks.http.targets` is a list of:

| Key | Type | Default | Meaning |
|---|---|---|---|
| `url` | string | — | The URL to probe. Required. |
| `expect_status` | int | `200` | Expected status code; a mismatch is BAD. |
| `max_latency_ms` | int | — | WARN if the response is slower. Omit to skip the latency check. |
| `expect_body` | string | — | BAD if this substring is absent from the body. Omit to skip. |

## `checks.nats`

NATS JetStream cluster health via the monitoring endpoints. See
[Modules → nats](modules.md#nats).

| Key | Type | Default | Meaning |
|---|---|---|---|
| `targets` | list | — | Monitoring endpoints as `host` or `host:port`. |
| `port` | int | `8222` | Default monitoring port for targets/inventory hosts without one. |
| `scheme` | string | `http` | `http` or `https` for the monitoring endpoint. |
| `ansible_inventory` | string | — | Ansible INI inventory; every host becomes a monitoring target on `port`. |
| `expect_meta_leader` | string | — | Expected meta-leader `server_name`; a mismatch is WARN. |
| `expect_peers` | list | — | Expected peer `server_name`s. Unexpected members → WARN (ghost); expected-but-absent → BAD. |
| `lag_warn` | int | `100` | Raft peer lag (entries) at/above which a peer is WARN. |
| `lag_crit` | int | `1000` | Raft peer lag (entries) at/above which a peer is BAD. |

```yaml
checks:
  nats:
    port: 8222
    expect_meta_leader: c0-nats-gw-01
    expect_peers: [sg-nats-gw-01, c0-nats-gw-01, ov-nats-gw-01]
    lag_warn: 100
    lag_crit: 1000
    targets:
      - 10.21.10.18
      - 10.11.10.18:8222
```

## `checks.haproxy`

HAProxy backend/server health via the CSV stats export. See
[Modules → haproxy](modules.md#haproxy).

| Key | Type | Default | Meaning |
|---|---|---|---|
| `targets` | list | — | Stats endpoints as `host` or `host:port`. |
| `port` | int | `8404` | Default stats port for targets/inventory hosts without one. |
| `scheme` | string | `http` | `http` or `https`. |
| `path` | string | `/stats;csv` | Path of the CSV stats export. |
| `ansible_inventory` | string | — | Ansible INI inventory; every host becomes a stats target on `port`. |
| `session_warn_pct` | int | `0` (off) | WARN when `scur/slim` reaches this percent. |
| `auth_user` | string | — | HTTP basic-auth user (optional). |
| `auth_pass_env` | string | — | Env var holding the basic-auth password. **Never put the password in the config.** |

```yaml
checks:
  haproxy:
    port: 8404
    path: /stats;csv
    session_warn_pct: 80
    auth_user: admin
    auth_pass_env: HAPROXY_STATS_PASS   # export HAPROXY_STATS_PASS=... in the environment
    targets:
      - 10.15.20.106:8404
```

## `checks.stream`

HLS/DASH stream health from the manifest. See
[Modules → stream](modules.md#stream).

`checks.stream.targets` is a list of:

| Key | Type | Default | Meaning |
|---|---|---|---|
| `url` | string | — | Manifest URL: HLS `.m3u8` (master or media) or DASH `.mpd`. Required. |
| `name` | string | the URL | Display label for the findings. |
| `min_variants` | int | `0` (skip) | Expected minimum ladder size (variants/representations). |
| `live` | bool | `false` | Expect a live stream: check live-edge freshness, WARN if it's VOD. |
| `max_age_warn_seconds` | int | `30` when `live` | Live-edge age → WARN. |
| `max_age_crit_seconds` | int | `60` when `live` | Live-edge age → BAD. |

```yaml
checks:
  stream:
    targets:
      - name: canale-live
        url: https://cdn.example/live/master.m3u8
        live: true
        min_variants: 3
      - name: vod-catalogo
        url: https://cdn.example/vod/movie/master.m3u8
        min_variants: 4
```

## `checks.patroni`

Patroni-managed PostgreSQL cluster health via the Patroni REST API. See
[Modules → patroni](modules.md#patroni).

| Key | Type | Default | Meaning |
|---|---|---|---|
| `targets` | list | — | Patroni REST endpoints as `host` or `host:port`. |
| `port` | int | `8008` | Default API port for targets/inventory hosts without one. |
| `scheme` | string | `http` | `http` or `https`. |
| `ansible_inventory` | string | — | Ansible INI inventory; every host becomes an API target on `port`. |
| `lag_warn_bytes` | int | `33554432` (32 MiB) | Replica lag → WARN. |
| `lag_crit_bytes` | int | `134217728` (128 MiB) | Replica lag → BAD. |

```yaml
checks:
  patroni:
    port: 8008
    lag_warn_bytes: 33554432
    lag_crit_bytes: 134217728
    targets:
      - 10.20.30.11
      - 10.20.30.12:8008
```

## `checks.consul`

Consul cluster health via the HTTP API. See [Modules → consul](modules.md#consul).

| Key | Type | Default | Meaning |
|---|---|---|---|
| `targets` | list | — | Consul HTTP API endpoints as `host` or `host:port`. |
| `port` | int | `8500` | Default API port for targets/inventory hosts without one. |
| `scheme` | string | `http` | `http` or `https`. |
| `ansible_inventory` | string | — | Ansible INI inventory; every host becomes an API target on `port`. |
| `expect_peers` | int | `0` (skip) | Expected raft peers; below quorum → BAD, below expected → WARN. |
| `token_env` | string | — | Env var holding the ACL token (sent as `X-Consul-Token`). **Never inline the token.** |
| `kv_keys` | list | — | KV keys that must exist; a missing key is BAD. |

```yaml
checks:
  consul:
    port: 8500
    expect_peers: 3
    token_env: CONSUL_HTTP_TOKEN
    kv_keys:
      - config/checkfleet/enabled
    targets:
      - 10.20.30.11
      - 10.20.30.12:8500
```

## `checks.postgres`

PostgreSQL health via read-only SQL. See [Modules → postgres](modules.md#postgres).

Top-level thresholds:

| Key | Type | Default | Meaning |
|---|---|---|---|
| `lag_warn_bytes` | int | `33554432` (32 MiB) | Replica lag → WARN. |
| `lag_crit_bytes` | int | `134217728` (128 MiB) | Replica lag → BAD. |
| `conn_warn_pct` | int | `80` | WARN when connections reach this % of `max_connections`. |
| `wraparound_warn_age` | int | `1500000000` | `age(datfrozenxid)` → WARN. |
| `wraparound_crit_age` | int | `1900000000` | `age(datfrozenxid)` → BAD. |
| `slot_warn_bytes` | int | `536870912` (512 MiB) | Inactive slot retained WAL → WARN. |
| `slot_crit_bytes` | int | `2147483648` (2 GiB) | Inactive slot retained WAL → BAD. |

`checks.postgres.targets` is a list of:

| Key | Type | Default | Meaning |
|---|---|---|---|
| `dsn` | string | — | libpq DSN or URL, **without the password**. Required. |
| `name` | string | the DSN | Display label for the findings. |
| `password_env` | string | — | Env var holding the password. **Never inline it.** |

```yaml
checks:
  postgres:
    conn_warn_pct: 80
    targets:
      - name: pg-prod-primary
        dsn: "host=10.20.30.11 port=5432 user=monitor dbname=postgres sslmode=require"
        password_env: PG_PROD_PASS
```

The monitoring role needs only read access (it queries `pg_stat_*`,
`pg_database`, `pg_replication_slots`, `pg_settings`).

## `checks.dns`

DNS resolution health. See [Modules → dns](modules.md#dns).

Top-level keys:

| Key | Type | Default | Meaning |
|---|---|---|---|
| `resolvers` | list | system | Resolvers as `host` or `host:port` (default port 53). Empty → `/etc/resolv.conf`. |
| `min_ttl_seconds` | int | `0` (off) | WARN when any answer's TTL is below this. |

`checks.dns.targets` is a list of:

| Key | Type | Default | Meaning |
|---|---|---|---|
| `name` | string | — | Domain to resolve. Required. |
| `type` | string | `A` | Record type: `A`, `AAAA`, `CNAME`, `NS`, `TXT`, `SOA`. |
| `expect` | list | — | Expected value set; a different answer is BAD (drift). For `SOA`, compared against the serial. |

```yaml
checks:
  dns:
    resolvers: [10.20.30.53, 8.8.8.8]
    min_ttl_seconds: 30
    targets:
      - {name: hiway.media, type: A, expect: ["203.0.113.10"]}
      - {name: hiway.media, type: SOA}
```

## Multi-stack profiles

`--stack <name>` overlays a per-stack file on the base config, so you can keep
one set of defaults and a small override per environment. Given
`--config checkfleet.yml --stack prod-cologno`, checkfleet loads
`checkfleet.yml` then overlays `checkfleet.prod-cologno.yml` (same directory).

The merge is **per module**: a module present in the stack file replaces the
base's module entirely (so the module gets its own defaults again); a module
absent from the stack is inherited from the base. `timeout_seconds` is
overridden only if the stack sets it. `--stack` works with both `check` and
`serve`.

```bash
checkfleet check all  --config checkfleet.yml --stack prod-cologno
checkfleet serve      --config checkfleet.yml --stack prod-cologno
```

```yaml
# checkfleet.prod-cologno.yml — overrides only what differs from the base
checks:
  certs:
    targets: [edge.cologno.hiway.media]
```

## `checks.redis`

Redis / Valkey health via `INFO`. See [Modules → redis](modules.md#redis).

| Key | Type | Default | Meaning |
|---|---|---|---|
| `targets` | list | — | Endpoints as `host` or `host:port`. |
| `port` | int | `6379` | Default port for targets/inventory hosts without one. |
| `tls` | bool | `false` | Use TLS (`rediss`). |
| `username` | string | — | Optional ACL username. |
| `password_env` | string | — | Env var holding the password. **Never inline it.** |
| `ansible_inventory` | string | — | Ansible INI inventory; every host becomes a target on `port`. |
| `mem_warn_pct` | int | `80` | WARN when `used_memory` reaches this % of `maxmemory`. |
| `lag_warn_bytes` | int | `16777216` (16 MiB) | Replica offset lag → WARN. |
| `lag_crit_bytes` | int | `134217728` (128 MiB) | Replica offset lag → BAD. |

```yaml
checks:
  redis:
    port: 6379
    password_env: REDIS_PASS
    mem_warn_pct: 80
    targets:
      - 10.20.30.40
      - 10.20.30.41:6380
```

## `checks.keycloak`

Keycloak health via HTTP. See [Modules → keycloak](modules.md#keycloak).

| Key | Type | Default | Meaning |
|---|---|---|---|
| `base_url` | string | — | Scheme + host (+ `/auth` prefix on old versions), no trailing slash. |
| `health_url` | string | — | Optional health endpoint (often on the management port `:9000`). Checked only when set. |
| `realms` | list | — | Realm names to verify via their OIDC discovery document. |

```yaml
checks:
  keycloak:
    base_url: https://auth.hiway.media
    health_url: https://auth.hiway.media:9000/health/ready
    realms: [hiway, partners]
```

## `checks.tcp`

Generic TCP reachability. See [Modules → tcp](modules.md#tcp).

`checks.tcp.targets` is a list of:

| Key | Type | Default | Meaning |
|---|---|---|---|
| `address` | string | — | `host:port` to connect to. Required. |
| `name` | string | the address | Display label. |
| `tls` | bool | `false` | TLS handshake instead of a plain connect. |
| `expect_banner` | string | — | Substring the server banner must contain. |
| `max_latency_ms` | int | — | WARN if the connect is slower. |

```yaml
checks:
  tcp:
    targets:
      - {name: ssh, address: 10.20.30.9:22, expect_banner: "SSH-2.0"}
      - {name: rtmp, address: ingest.hiway.media:1935}
```

## `checks.tls`

Deep TLS check. See [Modules → tls](modules.md#tls).

| Key | Type | Default | Meaning |
|---|---|---|---|
| `targets` | list | — | `host` or `host:port` (default 443). |
| `port` | int | `443` | Default port. |
| `warn_days` | int | `30` | Leaf expiry → WARN. |
| `crit_days` | int | `7` | Leaf expiry → BAD. |
| `ansible_inventory` | string | — | Ansible INI inventory; every host becomes a target. |

```yaml
checks:
  tls:
    targets: [auth.hiway.media, api.hiway.media:8443]
```

## `checks.ntp`

NTP clock offset. See [Modules → ntp](modules.md#ntp).

| Key | Type | Default | Meaning |
|---|---|---|---|
| `targets` | list | — | `host` or `host:port` (default 123). |
| `port` | int | `123` | Default port. |
| `offset_warn_ms` | int | `100` | \|offset\| → WARN. |
| `offset_crit_ms` | int | `1000` | \|offset\| → BAD. |

```yaml
checks:
  ntp:
    targets: [time.hiway.media, 0.pool.ntp.org]
```

## `checks.rabbitmq`

RabbitMQ management API. See [Modules → rabbitmq](modules.md#rabbitmq).

| Key | Type | Default | Meaning |
|---|---|---|---|
| `targets` | list | — | Management API endpoints `host` or `host:port`. |
| `port` | int | `15672` | Default management port. |
| `scheme` | string | `http` | `http` or `https`. |
| `username` | string | `guest` | Basic-auth user. |
| `password_env` | string | — | Env var holding the password. **Never inline.** |
| `queue_warn_depth` | int | `1000` | Queue messages → WARN. |
| `queue_crit_depth` | int | `50000` | Queue messages → BAD. |

```yaml
checks:
  rabbitmq:
    username: monitoring
    password_env: RABBITMQ_PASS
    targets: [10.20.30.60]
```

## `checks.grpc`

gRPC health checking (TLS/h2). See [Modules → grpc](modules.md#grpc).

`checks.grpc.targets` is a list of:

| Key | Type | Default | Meaning |
|---|---|---|---|
| `address` | string | — | `host:port` of the gRPC TLS endpoint. Required. |
| `name` | string | address | Display label. |
| `service` | string | — | gRPC service to check; empty = whole-server. |
| `insecure_skip_verify` | bool | `false` | Skip TLS cert verification (internal self-signed). |

```yaml
checks:
  grpc:
    targets:
      - {name: api, address: api.hiway.media:443, service: hiway.api.v1.API}
```

## `checks.ldap`

LDAP bind + search. See [Modules → ldap](modules.md#ldap).

`checks.ldap.targets` is a list of:

| Key | Type | Default | Meaning |
|---|---|---|---|
| `url` | string | — | `ldap://host:389` or `ldaps://host:636`. Required. |
| `name` | string | url | Display label. |
| `start_tls` | bool | `false` | StartTLS on a plain connection. |
| `insecure_skip_verify` | bool | `false` | Skip TLS cert verification. |
| `bind_dn` | string | — | Bind DN; empty = anonymous. |
| `password_env` | string | — | Env var with the bind password. **Never inline.** |
| `base_dn` | string | — | Search base for the sanity search. |
| `filter` | string | `(objectClass=*)` | Search filter. |
| `min_entries` | int | `1` (when base_dn set) | Minimum results, else BAD. |

## No secrets in config

Keep credentials out of `checkfleet.yml` — checks never log or echo secrets, and
example/config files must stay clean.
