---
title: Configuration
---

[← back to index](index.md)

# Configuration

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
| `timeout_seconds` | int | `30` | Deadline for the entire run. Findings still in flight are cut off. |
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

## No secrets in config

Keep credentials out of `checkfleet.yml` — checks never log or echo secrets, and
example/config files must stay clean.
