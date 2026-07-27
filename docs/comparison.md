---
title: Why checkfleet
nav_order: 10
description: >-
  How checkfleet compares to Prometheus, Blackbox exporter, Nagios/Icinga,
  Zabbix, Uptime Kuma and hand-written bash — what it replaces, what it
  complements, and when not to use it.
keywords:
  - checkfleet vs prometheus
  - blackbox exporter alternative
  - nagios alternative CLI
  - uptime kuma alternative
  - agentless health checks
  - domain-aware monitoring
---

**In one sentence:** checkfleet answers *domain* questions about infrastructure
on demand — "is the JetStream meta-leader elected?", "does every host in this
Ansible inventory have a valid certificate for the next 30 days?" — from a
single binary, with no server to run and nothing installed on the targets.

It is not a replacement for a metrics stack. It is the layer that a metrics
stack cannot express, and it hands the results back to that stack when you want
them there.

## The gap it fills

Generic monitoring answers *is it up?* well. It answers *is it correct?* badly,
because correctness is domain knowledge:

| Generic monitoring sees | The domain question | checkfleet module |
|---|---|---|
| Port 4222 is open | Is a JetStream meta-leader elected, and are peers current? | `nats` |
| PostgreSQL accepts connections | Is a replication slot inactive and retaining WAL? Is wraparound age climbing? | `postgres` |
| Patroni's HTTP port answers | Is there exactly one leader, and are replicas on the leader's timeline? | `patroni` |
| The manifest URL returns 200 | Is the bitrate ladder complete and is the live edge fresh? | `stream` |
| HAProxy's stats page loads | Does any backend have zero available servers? | `haproxy` |
| TLS handshake succeeds | Do all 300 hosts in the inventory still have 30 days of validity? | `certs` |
| Kafka brokers respond | Are partitions under-replicated and which consumer group is lagging? | `kafka` |

Encoding those questions in a metrics stack means an exporter per system, a
recording rule per signal, and an alert per threshold. checkfleet encodes them
once, in Go, and runs them anywhere.

## Compared to…

### Prometheus + Alertmanager

**Complement, not competitor.** Prometheus is a time-series database with
continuous scraping, retention, and alert routing. checkfleet is a one-shot,
stateless interrogation. Use Prometheus for trends, SLOs and paging; use
checkfleet for the domain assertions that are awkward as PromQL, and for
pre-flight checks before a deploy or a maintenance window.

If you want the findings *in* Prometheus, `checkfleet serve --listen :9876`
exposes them at `/metrics`. Scrape that and your existing alerting keeps
working — you have added domain knowledge, not a parallel stack.

### Blackbox exporter

Blackbox probes are the closest overlap, and checkfleet's `http`, `tcp`, `tls`,
`dns` and `grpc` modules cover similar ground. The differences: checkfleet runs
without a Prometheus deployment, reads targets straight from an Ansible
inventory, and — beyond those five modules — goes well past probing into cluster
state (`nats`, `patroni`, `consul`, `kafka`, `elasticsearch`, `etcd`, `vault`).
Blackbox stays the better choice if everything you need is a probe and you
already run Prometheus.

### Nagios / Icinga

The check-plugin model is the ancestor of this idea, and it works. What
checkfleet changes: one static binary instead of a plugin directory and a
scheduler daemon; YAML instead of per-host object definitions; no central server
to keep alive; and reports designed to be pasted into a runbook or a pull
request rather than rendered in a web UI. If you already operate an Icinga
estate and it fits, there is no reason to move.

### Zabbix, Datadog, New Relic

Agent-based platforms with discovery, storage, dashboards and paging. They do
far more than checkfleet and cost accordingly — in licence fees, in agents on
every host, or both. checkfleet has no agent, no account, and no telemetry: it
runs where you run it and prints what it found. Teams typically keep the
platform and add checkfleet for the domain checks and for CI gating.

### Uptime Kuma / Healthchecks.io

Excellent at external uptime and heartbeat monitoring with a friendly UI, and
they run as a service you host. checkfleet is a CLI with no persistent process,
aimed at internal correctness rather than public availability — and it goes
deeper than an HTTP status code.

### Hand-written bash and `check_*` scripts

This is the honest baseline, and the thing checkfleet is most often replacing. A
folder of `curl | jq` one-liners works until it needs thresholds, retries,
timeouts, concurrency, consistent exit codes, JSON output, and a second
maintainer. checkfleet is that folder, with tests, one release binary, and a
config a colleague can read.

## When *not* to use checkfleet

Being clear about this is cheaper than a disappointed evaluation:

- **You need history, graphs, or trend analysis.** checkfleet stores nothing
  (the desktop app keeps a local run history, but that is not a TSDB). Use
  Prometheus, VictoriaMetrics, or your platform.
- **You need paging with escalation and on-call rotations.** checkfleet emits
  findings; Alertmanager, PagerDuty or Opsgenie route them.
- **You need sub-minute continuous evaluation.** checkfleet runs when invoked.
  Cron every five minutes is a normal cadence; per-second scraping is not what
  it is for.
- **Your estate is fully managed SaaS with no internal services.** There is
  little domain state left for it to interrogate.
- **You need an OSI-approved open-source licence for commercial use.**
  checkfleet is source-available under PolyForm Noncommercial; commercial use
  needs a [separate licence]({{ site.github.repo }}/blob/main/COMMERCIAL.md).

## Where it fits in a working setup

- **Pre-deploy gate in CI** — `checkfleet check all --exit-on bad` before a
  release job touches production.
- **Weekly certificate sweep** — the `certs` module over the Ansible inventory,
  `--output markdown`, pasted into the ops channel or an issue.
- **Incident triage** — one command that asks every domain question at once,
  worst finding first, instead of ten terminal tabs.
- **Maintenance-window pre-flight and post-flight** — the same command before
  and after, with the two reports diffed.
- **Fed back into Prometheus** — `checkfleet serve` so the domain signals live
  next to everything else you already alert on.

Next: [Installation](installation.md) · [Modules](modules.md) · [FAQ](faq.md)
