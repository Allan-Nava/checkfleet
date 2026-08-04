---
name: checkfleet
description: Run domain-specific infrastructure checks with the checkfleet CLI and read its results correctly. Use when asked whether TLS certificates are expiring, whether a fleet of hosts is healthy, to probe HTTP/DNS/TCP endpoints, or to check Postgres, Redis, NATS, Kafka, Consul, HAProxy, Patroni, MongoDB, MySQL, RabbitMQ, Elasticsearch, Vault, etcd, ClickHouse, memcached, Cassandra, LDAP, SMTP, S3, gRPC, NTP or Keycloak — and when gating CI on infrastructure health.
---

# checkfleet

A single Go binary that runs a fleet of pluggable checks against real
infrastructure and reports findings. It does **not** replace Prometheus or
Grafana: it does the checks that need domain knowledge (is this cert expiring,
does this Patroni cluster have a leader, is this HLS live edge stale) and leaves
graphing and alerting to the tools that do those well.

## Commands

```bash
checkfleet check all --config checkfleet.yml            # run every configured module
checkfleet check certs --config checkfleet.yml          # one module
checkfleet validate --config checkfleet.yml             # config is well-formed, no run
checkfleet doctor --config checkfleet.yml               # preflight: unset ${ENV}, unreachable hosts
checkfleet explain redis                                # what a module checks, and its thresholds
checkfleet targets --config checkfleet.yml              # what is actually covered
checkfleet init --modules certs,http                    # scaffold a starter config
```

`check` also takes `--output`, `--only`, `--target`, `--min-severity`,
`--history`, `--baseline`, `--exit-on` and `--exit-code`. Run `checkfleet` with
no arguments for the full usage.

## Two rules that decide whether you read the output correctly

**1. Exit code 0 does not mean "everything is healthy."** A check that ran is a
success, even when it found something broken. `checkfleet check` exits 0 with
BAD findings; a non-zero exit means checkfleet itself failed (unreadable config,
unknown module, bad flag). To make findings fail a pipeline you must ask for it:

```bash
checkfleet check all --config checkfleet.yml --exit-on bad   # exits 2 on BAD or worse
```

Never infer health from the exit code alone.

**2. `ERROR` is not `BAD`.** `BAD` means the target is unhealthy. `ERROR` means
the check *could not measure* — network failure, TLS handshake refused, a broken
DSN. An `ERROR` is a statement about the probe, not about the target: reporting
"the database is down" from an `ERROR` is wrong, the honest reading is "we could
not tell." The four statuses are `OK`, `WARN`, `BAD`, `ERROR`.

## Read the JSON, do not grep the text

The text renderer is for humans and its wording is explicitly **not** a stable
interface — finding messages get reworded between releases. Use JSON:

```bash
checkfleet check all --config checkfleet.yml --output json | jq -r .worst
```

`worst` is the single field to gate on. Each finding carries `check`, `target`,
`status`, `message`, and optionally `value`/`unit` and `runbook`/`remediation`.
A finding is identified by the pair **`check` + `target`** — that is the
deduplication key everywhere (issues, alerts, history, baselines).

## References

Load these only when you need them:

- `references/modules.md` — every module, what it detects, and when to reach for
  it. Read this to pick a module or to answer "can checkfleet see X?".
- `references/config-schema.md` — config keys, types and defaults. Read this
  before writing or editing a `checkfleet.yml`.

## Constraints

- **No secrets in the config.** Credentials come from the environment:
  `password_env: PG_PASSWORD`, or `${VAR}` interpolation. Never write a literal
  password, token or key into `checkfleet.yml`, and never into a `runbooks:`
  note — those travel into every output, including Slack and issue trackers.
- checkfleet only reads. No module writes to the systems it checks.
