---
title: FAQ
nav_order: 11
description: >-
  Frequently asked questions about checkfleet — what it is, what it costs, how
  it differs from Prometheus, whether it needs an agent, and how it handles
  credentials.
keywords:
  - what is checkfleet
  - checkfleet license
  - checkfleet vs prometheus
  - agentless monitoring CLI
  - TLS expiry monitoring
faq:
  - q: What is checkfleet?
    a: >-
      checkfleet is a source-available command-line tool that runs domain-aware
      infrastructure health checks from a single static Go binary. You describe
      your targets in a `checkfleet.yml` file and run `checkfleet check all`;
      each module knows what "healthy" means for one kind of system — TLS
      certificates, HTTP endpoints, NATS, Kafka, PostgreSQL, Consul, HAProxy,
      HLS/DASH streams and more — and reports findings as text, Markdown, JSON,
      a Slack message, or Prometheus metrics.
  - q: How is checkfleet different from Prometheus?
    a: >-
      Prometheus collects and stores time series and alerts on them
      continuously; checkfleet answers a domain question on demand, in one shot,
      with no server and no storage. Prometheus can tell you a NATS node is up;
      checkfleet tells you the JetStream meta-leader is missing, two peers are
      lagging, and one is a ghost. They are complements, not alternatives —
      `checkfleet serve` exposes the findings as Prometheus metrics so alerting
      and dashboards stay where they already are.
  - q: Does checkfleet need an agent or a server?
    a: >-
      No. There is nothing to install on the targets and nothing to keep
      running. checkfleet is one static binary that executes the checks from
      wherever you invoke it — your laptop, a cron job, or a CI runner — using
      the same protocols and read-only endpoints an operator would use by hand.
  - q: Is checkfleet free? What licence does it use?
    a: >-
      checkfleet is source-available under the PolyForm Noncommercial License
      1.0.0. It is free for personal, research, educational, nonprofit and
      government use. Commercial use requires a separate licence — see
      COMMERCIAL.md in the repository. Releases published before v0.50.0 remain
      under the MIT licence they shipped with. It is not an OSI-approved
      open-source licence.
  - q: Can I use checkfleet at work? It is only internal, we do not resell it.
    a: >-
      If the company is for-profit, that is commercial use and needs a licence —
      even when checkfleet never leaves your own infrastructure and no customer
      ever sees it. "Commercial" in the PolyForm Noncommercial License is about
      the purpose the software serves, not about redistribution, so monitoring
      the systems that run a for-profit business is covered by it. Evaluating
      checkfleet before deciding is explicitly free, and so is asking whether
      your case qualifies. Nonprofits, charities, public research, public
      safety/health, environmental and government organisations are free
      regardless of budget. The details, and how to request a licence, are in
      COMMERCIAL.md.
  - q: How do I install checkfleet?
    a: >-
      Run `go install github.com/Allan-Nava/checkfleet/cmd/checkfleet@latest`,
      or `brew install Allan-Nava/tap/checkfleet` on macOS, or download a
      release archive for Linux, macOS or Windows from GitHub Releases. There is
      also a Docker image and a composite GitHub Action.
  - q: Why does checkfleet exit 0 when a check reports BAD?
    a: >-
      Because a check that ran successfully is a successful run — the exit code
      reports whether checkfleet worked, not whether your infrastructure is
      healthy. Non-zero exit codes are reserved for systemic failures such as an
      unreadable config or an unknown module. To fail a CI job on findings, pass
      `--exit-on warn`, `--exit-on bad` or `--exit-on error`.
  - q: What is the difference between the BAD and ERROR statuses?
    a: >-
      `BAD` means the target is unhealthy and checkfleet measured it. `ERROR`
      means checkfleet could not measure at all — a TCP connection refused, a
      TLS handshake failure, a timeout. Keeping them apart stops a broken
      network path from being reported as a broken service.
  - q: Can checkfleet monitor a whole fleet without listing every host?
    a: >-
      Yes. Point a module at an Ansible INI inventory with `ansible_inventory`
      and every host in it becomes a target, honouring `ansible_host`
      overrides. It is the fastest way to get TLS expiry coverage across an
      estate you already describe in Ansible.
  - q: How does checkfleet handle credentials and secrets?
    a: >-
      Credentials are read from environment variables referenced by name in the
      config (for example `token_env`), never stored in the YAML file. No check
      ever logs a credential, and no example config, test or document in the
      repository contains a real secret.
  - q: Which systems can checkfleet check?
    a: >-
      TLS certificates, HTTP endpoints, DNS, NTP, generic TCP services, gRPC
      health, LDAP, SMTP relays, S3-compatible object storage, RTMP/SRT ingest,
      HLS/DASH streams, NATS JetStream, Apache Kafka, RabbitMQ, Redis/Valkey,
      memcached, PostgreSQL, Patroni, MySQL/MariaDB, MongoDB, Cassandra/ScyllaDB,
      ClickHouse, Elasticsearch/OpenSearch, etcd, Consul, HashiCorp Vault,
      Keycloak and HAProxy.
  - q: Can I use checkfleet in CI?
    a: >-
      Yes, that is a primary use case. Run it as a step with
      `--exit-on bad` to fail the pipeline on unhealthy findings, or on a
      schedule with `--output markdown` to post a report. The repository ships a
      composite GitHub Action and a GitLab CI snippet.
  - q: Does checkfleet write to or modify the systems it checks?
    a: >-
      No. Every module uses read-only endpoints, read-only SQL, or a protocol
      handshake. The SMTP module never sends mail, the PostgreSQL and MySQL
      modules never run DDL or writes, and the cluster modules read status
      endpoints rather than administrative ones.
  - q: Is there a GUI?
    a: >-
      Yes. A desktop application built with Wails wraps the same engine and adds
      a status dashboard, run history, trends, grouping and muting, for macOS,
      Linux and Windows.
  - q: How do I add a check for something checkfleet does not support yet?
    a: >-
      Implement the `engine.Check` interface in a new package under
      `internal/checks/<name>`, add its typed configuration, wire it into the
      CLI, and test it against a local fixture server — the test suite never
      touches the network or real infrastructure. The Development page walks
      through it.
---

Short answers to the questions people actually ask before adopting checkfleet.
If yours isn't here, open a
[discussion or issue]({{ site.github.repo }}/issues).

{% for item in page.faq %}
## {{ item.q }}

{{ item.a | markdownify }}
{% endfor %}

## Still deciding?

Read [why checkfleet exists and how it compares](comparison.md) to Prometheus,
Blackbox exporter, Nagios and friends, or jump straight to
[Installation](installation.md).

## Can a finding leak a password?

No, and it is verified rather than promised. A test in `internal/registry`
configures **every** credentialed module with a sentinel value in each password
and token field, points them at a dead port so they all fail the way they fail
in production, and asserts that no finding — message, target, runbook or
remediation — contains it. That test can fail: a companion case feeds it a
finding that does leak and checks it is rejected.

Behind it, `engine.Redact` strips passwords out of connection strings and error
text, and the three modules whose DSN carries the credential inline (`mysql`,
`mongodb`, `postgres`) run their connection errors through it — a driver error
that echoes its input is the classic way a password reaches a log.

## Can checkfleet change anything on the systems it checks?

No, and this is enforced in two directions rather than promised.

**Statically**: a guard walks every module source and fails the build if one
issues `POST`, `PUT`, `DELETE` or `PATCH`, or sends a mutating command in the
hand-rolled text protocols (`SET`, `DEL`, `FLUSHALL`, …). Two files are allowed
POST with a written reason, and both are transport rather than mutation:
etcd's gRPC-JSON gateway requires POST even for reads, and gRPC itself is POST
over HTTP/2. Adding a third exception means arguing for it in review.

**Dynamically**: the integration suite runs the driver-backed checks
(`postgres`, `mysql`, `mongodb`) as the least-privilege accounts from
[Permissions](permissions.md) — `pg_monitor`, `PROCESS`+`REPLICATION CLIENT`,
`clusterMonitor` — none of which carries `INSERT`, `UPDATE`, `DELETE` or DDL.
The suite first proves the account really is refused a `CREATE TABLE`, because a
test that runs as a user who happens to have rights proves nothing, and then
asserts the checks pass anyway.
