---
title: Modules
---

[← back to index](index.md)

# Modules

Each module is a self-contained check that knows what "healthy" means for one
kind of target. Shipping today: `certs`, `http`, `nats`. The
[backlog](https://github.com/Allan-Nava/checkfleet/blob/main/BACKLOG.md) tracks
what's next (`haproxy`, `stream`, `patroni`, `postgres`, `consul`, …).

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

## Ansible inventory as a target source

The `certs` and `nats` modules can read a standard Ansible **INI** inventory (a
file or a directory of files):

- host lines and their `ansible_host=` value are used;
- `:vars` and `:children` sections are ignored;
- hosts are de-duplicated.

Every discovered host becomes a target on the module's `port` (443 for `certs`,
8222 for `nats`).
