---
title: Configuration
nav_order: 3
description: >-
  The complete checkfleet.yml reference — global timeouts and retries, every
  module's options and thresholds, stack overlays, and secrets from the
  environment.
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
| `max_concurrency` | int | `0` (unbounded) | Cap on how many checks run at once. Handy for large fleets so a run doesn't open hundreds of connections at once. `--max-concurrency` overrides it. |
| `labels` | map | — | Global key/value labels attached to the outputs — see [Global labels](#global-labels). |
| `module_overrides` | map | — | Per-module override of `timeout_seconds`/`retries`/`retry_backoff_ms`, keyed by module name. A zero field falls back to the global value. |
| `include` | list | — | Files or directories deep-merged under this config at load time — see [Splitting config across files](#splitting-config-across-files-include). |
| `checks` | map | — | One entry per module. A module runs only if its key is present. |

Per-module tuning is handy when one module is slower or flakier than the rest —
give a database check a shorter deadline and a couple of retries without changing
the global settings:

```yaml
timeout_seconds: 30
module_overrides:
  postgres: {timeout_seconds: 10, retries: 2}
  stream:   {timeout_seconds: 15}
```

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
    expect_meta_leader: nats-gw-01
    expect_peers: [nats-gw-01, nats-gw-02, nats-gw-03]
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
      - {name: example.com, type: A, expect: ["203.0.113.10"]}
      - {name: example.com, type: SOA}
```

## Multi-stack profiles

`--stack <name>` overlays a per-stack file on the base config, so you can keep
one set of defaults and a small override per environment. Given
`--config checkfleet.yml --stack prod`, checkfleet loads
`checkfleet.yml` then overlays `checkfleet.prod.yml` (same directory).

The merge is **per module**: a module present in the stack file replaces the
base's module entirely (so the module gets its own defaults again); a module
absent from the stack is inherited from the base. Every module can be overridden
this way. `timeout_seconds` is overridden only if the stack sets it. `--stack`
works with both `check` and `serve`.

**Compose several stacks** by passing a comma-separated list — they overlay
**left-to-right, last wins**, so you can layer environment on region on base:

```bash
checkfleet check all  --config checkfleet.yml --stack prod
checkfleet check all  --config checkfleet.yml --stack region-eu,prod   # prod wins
checkfleet serve      --config checkfleet.yml --stack region-eu,prod
```

Each stack file resolves its own [`include`](#splitting-config-across-files-include)
before it overlays the base.

```yaml
# checkfleet.prod.yml — overrides only what differs from the base
checks:
  certs:
    targets: [edge.example.com]
```

## Global labels

`labels:` attaches key/value metadata to a run — which environment, region,
cluster — and carries it into the outputs for routing and dashboards:

```yaml
labels:
  env: prod
  region: eu
```

Where they surface:

| Output | How labels appear |
|---|---|
| `prometheus` | on **every series** — `checkfleet_finding_status{check="…",target="…",env="prod",region="eu"}` (invalid label chars in a key become `_`). |
| `json` | a top-level `"labels": { … }` object. |
| `otlp` | as **resource attributes** on the metrics. |
| `webhook` (template) | available as `.Labels` (e.g. `{{ .Labels.env }}`). |

Labels are **operator metadata only** — never secrets. Other formats (text,
markdown, …) ignore them.

## Splitting config across files (`include`)

A large fleet reads better split by team or service. `include:` pulls other
files (or a whole directory) into one config at load time:

```yaml
# checkfleet.yml
include:
  - conf.d/            # every *.yml / *.yaml in the directory, in sorted order
  - ../shared/dns.yml  # a single file, relative to THIS file
timeout_seconds: 30
checks:
  http:
    targets: [{url: https://app.example/health}]
```

Rules:

- **Paths are relative** to the file doing the include (absolute paths work
  too). A directory contributes its `*.yml`/`*.yaml` entries, sorted by name —
  prefix them `10-`, `20-`, … to control order.
- The merge is a **deep merge**: two files can each add different modules under
  `checks:`, and they combine. Redefining the *same* module (or any scalar/list
  like a module's `targets:`) **replaces** it wholesale.
- **The including file wins** over everything it includes; among includes, a
  **later entry wins** over an earlier one. Includes may nest.
- `${…}` interpolation runs per file, and an **include cycle** or a missing
  file is a clear load error.

`include` composes with `--stack`: the base and the stack file each resolve
their own includes before the stack overlays the base.

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
    base_url: https://auth.example.com
    health_url: https://auth.example.com:9000/health/ready
    realms: [main, partners]
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
      - {name: rtmp, address: ingest.example.com:1935}
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
    targets: [auth.example.com, api.example.com:8443]
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
    targets: [time.example.com, 0.pool.ntp.org]
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
      - {name: api, address: api.example.com:443, service: example.api.v1.API}
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

## `checks.kafka`

Kafka cluster health. See [Modules → kafka](modules.md#kafka).

| Key | Type | Default | Meaning |
|---|---|---|---|
| `brokers` | list | — | Seed brokers `host:port`. Required. |
| `tls` | bool | `false` | Dial over TLS. |
| `sasl_user` | string | — | SASL username (enables SASL). |
| `sasl_mechanism` | string | `plain` | `plain`, `scram-sha-256`, `scram-sha-512`. |
| `sasl_password_env` | string | — | Env var with the SASL password. **Never inline.** |
| `expect_brokers` | int | `0` | Fewer brokers than this → WARN. |
| `groups` | list | — | Consumer groups whose total lag to check. |
| `lag_warn` | int | `1000` | Group lag → WARN. |
| `lag_crit` | int | `100000` | Group lag → BAD. |

```yaml
checks:
  kafka:
    brokers: [10.20.30.70:9092]
    expect_brokers: 3
    groups: [ingest-consumers]
```

## No secrets in config

Keep credentials out of `checkfleet.yml` — checks never log or echo secrets, and
example/config files must stay clean.

## Dynamic values & secrets

Config values support `${…}` interpolation, expanded before the file is parsed:

| Token | Expands to |
|---|---|
| `${VAR}` | environment variable `VAR` (empty if unset) |
| `${VAR:-default}` | `VAR`, or `default` when unset/empty |
| `${file:/path}` | the trimmed contents of a file — Docker/Kubernetes secrets |

```yaml
timeout_seconds: ${CF_TIMEOUT:-30}
checks:
  redis:
    targets: ["${REDIS_HOST}:6379"]
    password_env: REDIS_PASSWORD          # module secrets still come from env…
  postgres:
    targets:
      - name: primary
        dsn: "postgres://app:${file:/run/secrets/pg_password}@db:5432/app"
```

A missing `${file:…}` is a hard error. Use `$${` for a literal `${`. This keeps
secrets out of `checkfleet.yml` while staying friendly to `*_env` module fields.

## Maintenance windows

Suppress or downgrade findings during planned work so they don't page. Each
window matches by `check`/`target` glob (empty = all) and an optional
`from`/`to` (RFC3339) range; the first matching, active window wins.

```yaml
maintenance:
  - check: postgres            # mute (drop) all postgres findings in the range
    from: 2026-08-01T22:00:00Z
    to:   2026-08-01T23:30:00Z
  - target: "cdn.*"            # keep visible but cap BAD/ERROR at WARN
    action: warn               # message gets a " [maintenance]" note
```

`action: mute` (default) drops the finding; `action: warn` caps `BAD`/`ERROR` at
`WARN`. Applies to `check` (before `--exit-on-bad`) and to `serve`.

**Recurring windows** — add `daily: "HH:MM-HH:MM"` (local clock, wraps past
midnight) for a window that repeats every day, optionally restricted to
`weekdays`. `from`/`to` still bound the overall validity (e.g. a nightly window
only for one month).

```yaml
maintenance:
  - check: postgres            # every night 01:00–03:00 local, muted
    daily: "01:00-03:00"
  - target: "cdn.*"            # only on weekends, capped at WARN
    daily: "00:00-23:59"
    weekdays: [Sat, Sun]
    action: warn
```

## Runbooks and remediation hints

A finding tells you *what* is wrong. `runbooks:` attaches *what to do about it*:
a procedure URL and a short note, carried into the outputs so whoever is on call
does not have to go and find the wiki page.

Rules match like maintenance windows — `check` and `target` globs, empty meaning
all — and are read in order.

```yaml
runbooks:
  - check: certs
    runbook: https://wiki.example.com/runbooks/tls-renewal
    remediation: Renew with certbot, then reload haproxy
  - check: postgres
    target: "db-*"
    remediation: Check replication lag before failing over
  - runbook: https://wiki.example.com/runbooks/oncall   # catch-all
```

The first non-empty value wins **per field**, so a specific rule can supply the
runbook while a catch-all below it still supplies the remediation — as in the
example above, where a `certs` finding gets both from the first rule but a
`redis` finding gets only the catch-all URL.

Hints are attached only to findings above `OK`: there is nothing to do about a
green result, and repeating the URL on every healthy target is noise.

Where they show up:

| Output | How |
|---|---|
| `text` | an indented `↳ note — url` line under the finding |
| `markdown` | a second line in the Detail cell of **Needs attention**, runbook as a link |
| `json` | the `runbook` and `remediation` fields, omitted when unset |
| `html` | a muted line under the message, runbook as a link |
| desktop | a **What to do** block in the finding detail drawer |

> **No secrets.** This is operational text that travels into every output,
> including the ones that leave the host (Slack, webhooks, issue trackers). Put
> a URL and a sentence here — never a token, a password or an internal
> credential path.

## Client certificates (mTLS)

Where a fleet requires mutual TLS, the affected checks need an identity of their
own. Six modules take it from the config — `http`, `grpc`, `tcp`, `smtp`,
`elasticsearch`, `kafka` — with the same three keys:

```yaml
checks:
  http:
    client_cert: /etc/checkfleet/client.crt
    client_key:  /etc/checkfleet/client.key
    ca_cert:     /etc/checkfleet/ca.crt      # verify the server against this
    targets:
      - {url: "https://api.internal/health", expect_status: 200}
```

**Paths only, never inline PEM.** A private key pasted into `checkfleet.yml`
would be a secret in a config file, which the no-secrets rule forbids.

Naming a `ca_cert` **turns server verification back on** for the modules that
skip it by default (`tcp`, `smtp`): a CA you configured on purpose that was then
ignored would make the setting decorative.

A half-configured pair — `client_cert` without `client_key`, or the reverse — is
an error naming the missing half, not a silent fall back to no certificate. The
silent version turns a typo into an hour spent asking why the server rejects
you.

The three driver-backed modules take the same thing through their own
connection string, and deliberately do **not** repeat these keys — a second way
to say it is a knob that can disagree with the first:

| Module | Where |
|---|---|
| `postgres` | `sslcert=`, `sslkey=`, `sslrootcert=` in the DSN |
| `mongodb` | `tlsCertificateKeyFile=`, `tlsCAFile=` in the URI |
| `mysql` | the driver's TLS parameters in the DSN |

## File permissions

`checkfleet doctor` warns when the config, or a file it reads a secret from with
`${file:...}`, is readable by other accounts on the host:

```
🟡 WARN  doctor/perms  checkfleet.yml  the config is world-readable (mode 0644); run: chmod 0600 checkfleet.yml
```

A `checkfleet.yml` matters even when it holds only `*_env` keys: it maps the
fleet, names every host and port, and says which credential lives in which
variable. That is a reconnaissance document.

`doctor` **warns and moves on** — a permission that is wrong today should not
take your monitoring down with it, since the run is how you find out something
else broke.

**One case is refused rather than warned about**: reading a *world-readable*
file as a secret with `${file:...}`. There, continuing means using the
credential anyway, so the load fails with the `chmod` to run. Group-readable
(`0640`) is accepted: running under a dedicated group is a normal deployment,
and refusing it would push the password back into a unit file.

## Dependencies between checks

A dead host produces one finding per module that touches it, and `alert` opens
them all. `depends_on` says which findings are *consequences* of another, so the
run reports one outage instead of six:

```yaml
depends_on:
  # Everything on a host depends on that host answering on SSH.
  - on_check: tcp
    same_host: true

  # Or name the parent explicitly.
  - check: postgres
    target: "db-*"
    on_check: tcp
    on_target: "db-01:22"
```

When the parent is `BAD` or `ERROR`, its dependents are **downgraded to `WARN`
and annotated** — `[suppressed by tcp db-01:22]`, plus a `suppressed_by` field
in the JSON. A merely `WARN` parent suppresses nothing: it has not explained
its children away.

**Suppressed findings are never hidden.** A row that disappears is
indistinguishable from a check that never ran, and "the fleet went quiet" is the
worst way to learn about an outage. They stay on screen, marked, and below the
`--exit-on bad` gate.

{: .note }
> `same_host` compares the **host part of the target**, so both findings have to
> spell their target as `host` or `host:port`. A module configured with a
> friendly `name:` reports that name — `db-primary` shares no host with
> `10.0.0.5:5432` — and the rule then matches nothing. Use an explicit
> `on_target` for named targets.

A cycle (`a` depends on `b` depends on `a`) is refused by
[`validate`](usage.md) rather than resolved at run time, where the outcome would
depend on the order findings happened to arrive in.

## Alert routing

`checkfleet alert --provider X` sends the whole run to one place: either you
wake the wrong team or you route nothing. `alert_routes` sends each alert where
it belongs:

```yaml
alert_routes:
  - check: postgres
    provider: pagerduty
    key_env: PD_DBA_ROUTING_KEY

  - check: haproxy
    provider: opsgenie
    key_env: OG_NETWORK_KEY

  - min_severity: error          # anything that could not be measured
    labels: {env: prod}          # only in production
    provider: pagerduty
    key_env: PD_ONCALL

  - provider: sns                # catch-all, last
    sns_topic_arn: arn:aws:sns:eu-west-1:123456789:fleet
```

Rules are read in order and **the first match wins**, so specific rules go on
top and a catch-all — one with no match fields — goes at the bottom. `labels`
must *all* match the run's global labels. `key_env` names the variable holding
the key, never the key.

See where an alert would go before turning it on:

```
$ checkfleet alert --config checkfleet.yml --dry-run
  trigger postgres/db-01:5432        → pagerduty (PD_DBA_ROUTING_KEY)
  trigger http/https://api.internal/ → sns arn:aws:sns:eu-west-1:123456789:fleet
```

Two behaviours worth knowing:

- **A resolve is never filtered by `min_severity`.** It is the *end* of a
  problem and carries no severity; letting the filter swallow it would route the
  trigger to a team and leave the alert open there forever.
- **An event matching no rule is reported and skipped**, not sent somewhere
  arbitrary. A config with rules has opinions about where things go, and
  defaulting quietly would deliver a database alert to whoever happens to be
  first in the list.

`validate` refuses an unknown provider, a missing key, a bad `min_severity`, and
a rule placed **after** a catch-all where it can never fire — a mistake whose
symptom (alerts arriving in the wrong place) looks exactly like the routing not
working at all.

### Re-notifying about a problem that stays open

`alert` deduplicates by check+target, which is right for the first notification
and useless afterwards: a `BAD` that lasts three days either re-fires on every
run — so people mute the channel — or never fires again and is forgotten.
Neither is a decision anyone made.

```yaml
alert_routes:
  - provider: pagerduty
    key_env: PD_ONCALL
    renotify_after: 4h            # ping again while it is still open
    renotify_on_worsening: true   # and immediately if it gets worse
```

`renotify_after` needs somewhere to remember what was last sent:

```bash
checkfleet alert --config checkfleet.yml --alert-state /var/lib/checkfleet/alerts.json
```

That file is separate from `--history` on purpose: the history is a
[contractual format](compatibility.md) other things read, and notification
bookkeeping is neither interesting to them nor stable enough to freeze. It is
written atomically and with no group or other access.

`renotify_on_worsening` fires the moment the status deteriorates
(`WARN`→`BAD`→`ERROR`) regardless of the interval: a situation that got worse is
new information, and holding it for the timer is the wrong trade.

A **resolve always goes**, and clears the memory — so a problem that returns a
month later is a first notification again, not something judged against an
ancient timer.

`--dry-run` says what the policy did rather than leaving you to infer it from
silence:

```
  trigger tcp/10.0.0.5:22    · held (notified 12m ago, waiting for 4h0m0s)
```

With no `alert_routes`, the `alert` flags behave exactly as before, including
notifying once and staying quiet.
