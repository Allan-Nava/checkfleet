---
title: Modules
---

[← back to index](index.md)

# Modules

Each module is a self-contained check that knows what "healthy" means for one
kind of target. Shipping today: `certs` and `http`. The
[backlog](https://github.com/Allan-Nava/checkfleet/blob/main/BACKLOG.md) tracks
what's next (`nats`, `haproxy`, `stream`, `patroni`, `postgres`, `consul`, …).

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

## Ansible inventory as a target source

The `certs` module can read a standard Ansible **INI** inventory (a file or a
directory of files):

- host lines and their `ansible_host=` value are used;
- `:vars` and `:children` sections are ignored;
- hosts are de-duplicated.

Every discovered host becomes a certs target on the module's `port`.
