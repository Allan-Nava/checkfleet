<p align="center">
  <img src="docs/assets/logo.png" alt="checkfleet logo" width="116" height="116">
</p>

<h1 align="center">checkfleet</h1>

<p align="center"><strong>A fleet of <em>domain-aware</em> infrastructure checks in one Go binary.</strong></p>

<p align="center">
  <a href="https://github.com/Allan-Nava/checkfleet/releases"><img alt="Latest release" src="https://img.shields.io/github/v/tag/Allan-Nava/checkfleet?label=release&sort=semver&color=10b981"></a>
  <a href="https://github.com/Allan-Nava/checkfleet/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/Allan-Nava/checkfleet/actions/workflows/ci.yml/badge.svg"></a>
  <a href="LICENSE"><img alt="License: PolyForm Noncommercial 1.0.0" src="https://img.shields.io/badge/license-PolyForm%20Noncommercial%201.0.0-f59e0b"></a>
  <img alt="Go" src="https://img.shields.io/github/go-mod/go-version/Allan-Nava/checkfleet?color=10b981">
</p>

<p align="center">📖 <strong>Full documentation: <a href="https://allan-nava.github.io/checkfleet/">allan-nava.github.io/checkfleet</a></strong></p>

---

**checkfleet is a command-line tool that runs domain-aware infrastructure health checks from a single static Go binary.** You describe your targets in a `checkfleet.yml`, run `checkfleet check all`, and get findings as terminal output, an ops-style Markdown report, JSON, a Slack message, or Prometheus metrics. 29 modules ship today — TLS certificate expiry, HTTP, DNS, NATS, Kafka, PostgreSQL, MySQL, MongoDB, Redis, Consul, Vault, HAProxy, Elasticsearch, HLS/DASH streams and more.

No agent to install on the targets. No server to keep running. No account, no telemetry.

```
$ checkfleet check all --config checkfleet.yml

🔴 BAD   http     https://example.com/health   HTTP 404 (want 200), 151ms
🟢 OK    certs    example.com:443              expires in 41 days (2026-09-02, CN=*.example.com)
🟢 OK    http     https://example.com/         HTTP 200, 168ms

3 checks: 2 OK, 0 WARN, 1 BAD, 0 ERROR (in 227ms)
```

## Philosophy

Don't rebuild Prometheus or Grafana. checkfleet fills the layer they can't: checks that need **domain knowledge** (what "healthy" means for a TLS estate, a NATS cluster, an HLS stream), runnable from CI, cron, or your laptop, with reports you can paste straight into your ops docs.

- **Exit code 0 even on WARN/BAD findings** — a check that ran *is* a success. Gate on the output, or use `--exit-on warn|bad|error` for CI.
- **Worst findings first** — the thing you must look at is the first line.
- **Fleet-aware** — point the certs check at your Ansible inventory and every host becomes a target.

<details>
<summary><strong>Isn't this what Prometheus / Blackbox exporter / Nagios already do?</strong></summary>

<br>

Generic monitoring answers *is it up?* well and *is it correct?* badly, because correctness is domain knowledge:

| Generic monitoring sees | The domain question checkfleet answers |
|---|---|
| Port 4222 is open | Is a JetStream meta-leader elected, and are peers current? |
| PostgreSQL accepts connections | Is a replication slot inactive and retaining WAL? |
| The manifest URL returns 200 | Is the bitrate ladder complete and the live edge fresh? |
| TLS handshake succeeds | Do all 300 inventory hosts still have 30 days of validity? |

They're complements: `checkfleet serve` exposes the findings as Prometheus metrics, so your existing dashboards and alerting keep working. Full write-up — including **when *not* to use checkfleet** — in [Why checkfleet](https://allan-nava.github.io/checkfleet/comparison/), and short answers in the [FAQ](https://allan-nava.github.io/checkfleet/faq/).

</details>

## Install

```bash
go install github.com/Allan-Nava/checkfleet/cmd/checkfleet@latest
# or: brew install Allan-Nava/tap/checkfleet   (macOS; `brew tap Allan-Nava/tap` first if you prefer)
# or: download a release archive (tar.gz/zip + checksums.txt) from GitHub Releases
```

See [Installation](https://allan-nava.github.io/checkfleet/installation/) for all options.

## Modules

| Module | What it checks |
|---|---|
| `certs` | TLS certificate expiry (WARN/BAD thresholds in days) for explicit targets **and/or every host of an Ansible INI inventory** |
| `http` | HTTP probes: expected status, max latency (WARN), body substring |
| `nats` | NATS JetStream cluster health via `/varz` + `/jsz?meta=1`: meta-leader present/expected, offline or lagging peers, ghost/missing peers, mixed versions |
| `haproxy` | Backend/server health from the CSV stats export: servers DOWN/MAINT/DRAIN, backends with no available server, optional session saturation |
| `stream` | HLS/DASH stream health from the manifest: reachable & valid, complete bitrate ladder, live-edge freshness (live) |
| `patroni` | Patroni PostgreSQL cluster via the REST API: single leader, replica state, replica lag, timeline divergence |
| `consul` | Consul cluster via the HTTP API: raft leader & quorum, critical/warning health checks, required KV keys |
| `postgres` | PostgreSQL via read-only SQL: wraparound risk, connection saturation, inactive replication slots, replica lag |
| `dns` | DNS resolution via an in-tree client: records resolve, drift from expected, SOA-serial & answer consistency across resolvers, low TTL |
| `redis` | Redis/Valkey via an in-tree RESP client (INFO): reachability & role, memory vs maxmemory, replication link & lag, persistence (RDB/AOF) |
| `keycloak` | Keycloak via HTTP: health endpoint UP, per-realm OIDC discovery (token endpoint present, issuer coherent) |
| `tcp` | Generic TCP reachability: connect (optionally TLS), latency, optional banner match |
| `tls` | Deep TLS: chain validity, certificate expiry, weak negotiated protocol version |
| `ntp` | NTP clock offset & stratum via a hand-rolled SNTP query (drift breaks TLS/JWT) |
| `rabbitmq` | RabbitMQ via the management API: nodes running & alarm-free, queue depth & consumer presence |
| `grpc` | gRPC Health Checking Protocol over HTTP/2+TLS (protobuf hand-rolled, no gRPC dep) |
| `ldap` | LDAP directory: connect, bind (anon or creds), optional sanity search |
| `kafka` | Kafka via kadm: controller present, broker count, under-replicated partitions, consumer-group lag |
| `ingest` | Streaming ingest reachability: RTMP handshake (TCP) or SRT induction (UDP), connect latency — "can the streamer publish?" |
| `s3` | S3/object storage: bucket reachable, optional sentinel object present & fresh; AWS SigV4 signed by hand (creds from env) |
| `smtp` | SMTP relay: accepts connections, 220 greeting, EHLO, optional STARTTLS/implicit TLS and relay cert expiry — never sends mail |
| `elasticsearch` | Elasticsearch/OpenSearch via HTTP: cluster health green/yellow/red, unassigned shards, expected nodes, per-node disk watermark |
| `mongodb` | MongoDB via the official driver: replica-set status (primary present, member health, secondary lag) and connection saturation |
| `mysql` | MySQL/MariaDB via go-sql-driver: reachable, read-only role, connection saturation, replica IO/SQL threads + lag |
| `etcd` | etcd v3 via the HTTP JSON gateway: /health, leader present (quorum), member count vs expected |
| `clickhouse` | ClickHouse over HTTP: /ping, SELECT version(), replicated-table read-only state & replication delay |
| `vault` | HashiCorp Vault over HTTP: seal status (sealed/uninitialized), active/standby role, version |
| `memcached` | memcached over the text protocol: reachability, memory vs limit_maxbytes, evictions, connections, version |
| `cassandra` | Cassandra/ScyllaDB via the CQL native protocol handshake: node accepts CQL, handshake latency, cluster state vs expect_nodes |

The only module still on the roadmap (see [BACKLOG.md](BACKLOG.md)) is `mediamtx`.

## Configuration

```yaml
# checkfleet.yml
timeout_seconds: 30
retries: 2               # retry a check that ERRORs (network/handshake) before reporting
retry_backoff_ms: 250
checks:
  certs:
    warn_days: 30
    crit_days: 7
    port: 443
    targets:
      - example.com
      - internal.example:8443
    ansible_inventory: /path/to/inventory   # optional: every host → target
  http:
    targets:
      - url: https://example.com/
        expect_status: 200
        max_latency_ms: 2000
        expect_body: "ok"
```

Layer a `checkfleet.<stack>.yml` on top of the base with `--stack <name>` (per-module merge). See the [Configuration reference](https://allan-nava.github.io/checkfleet/configuration/).

## Usage

```bash
checkfleet init        --modules certs,http                      # scaffold a starter config
checkfleet check all   --config checkfleet.yml                    # terminal
checkfleet check certs --config checkfleet.yml --output markdown  # ops report
checkfleet check nats  --config checkfleet.yml --output markdown  # NATS cluster health
checkfleet check postgres --config checkfleet.yml                 # PostgreSQL (read-only SQL)
checkfleet check dns   --config checkfleet.yml                    # DNS resolution & drift
checkfleet check http  --config checkfleet.yml --output json      # machine-readable (includes "worst")
checkfleet check all   --config checkfleet.yml --exit-on bad       # exit 2 on BAD/ERROR, for CI gates
checkfleet check all   --config checkfleet.yml --output slack     # post a Block Kit report to a Slack webhook

# scope the findings
checkfleet check all --config checkfleet.yml --only certs,http    # run/report only these checks
checkfleet check all --config checkfleet.yml --min-severity warn  # hide OK
checkfleet check all --config checkfleet.yml --target 'example.*'  # glob on target

# other commands
checkfleet validate     --config checkfleet.yml                   # validate the config without running checks
checkfleet doctor       --config checkfleet.yml                   # preflight: unset ${ENV}, bad targets, unreachable hosts
checkfleet targets      --config checkfleet.yml                   # what is covered, across every module
checkfleet targets      --config checkfleet.yml --against hosts.ini  # which inventory hosts are unmonitored
checkfleet serve        --config checkfleet.yml --listen :9876    # Prometheus exporter (metrics at /metrics)
checkfleet report-issues --config checkfleet.yml                  # open/close GitHub issues from BAD findings
```

Finding statuses: `OK`, `WARN` (threshold crossed), `BAD` (target unhealthy), `ERROR` (the check itself could not measure — network, handshake).

## Development

```bash
go test ./...    # all tests run against local in-test servers — no network needed
go vet ./...
go build -o checkfleet ./cmd/checkfleet
```

Adding a module: implement `engine.Check` in `internal/checks/<name>`, add its typed config in `internal/engine/config.go`, wire it in `cmd/checkfleet/main.go`, and test it against a local fixture server.

**Opt-in integration suite** — the unit tests above stay offline; a separate,
tag-gated suite exercises the modules against real services in Docker:

```bash
docker compose -f docker-compose.integration.yml up -d --build --wait
go test -tags integration ./test/integration/...
docker compose -f docker-compose.integration.yml down -v
```

It never runs under `go test ./...`. CI runs it in its own workflow
(`.github/workflows/integration.yml`), separate from the unit-test job.

## Documentation

Full docs: **[allan-nava.github.io/checkfleet](https://allan-nava.github.io/checkfleet/)**

[Installation](https://allan-nava.github.io/checkfleet/installation/) ·
[Configuration reference](https://allan-nava.github.io/checkfleet/configuration/) ·
[Usage](https://allan-nava.github.io/checkfleet/usage/) ·
[Modules](https://allan-nava.github.io/checkfleet/modules/) ·
[Output formats](https://allan-nava.github.io/checkfleet/output/) ·
[Desktop app](https://allan-nava.github.io/checkfleet/desktop/) ·
[CI integration](https://allan-nava.github.io/checkfleet/ci/) ·
[Why checkfleet](https://allan-nava.github.io/checkfleet/comparison/) ·
[Compatibility](https://allan-nava.github.io/checkfleet/compatibility/) ·
[FAQ](https://allan-nava.github.io/checkfleet/faq/)

Reading this as a language model? [`/llms.txt`](https://allan-nava.github.io/checkfleet/llms.txt)
is a structured index of the project and
[`/llms-full.txt`](https://allan-nava.github.io/checkfleet/llms-full.txt) is the
whole documentation set as plain text.

## Contributing

Bug reports, module proposals and PRs are welcome — [CONTRIBUTING.md](CONTRIBUTING.md) has the gate (`go vet` + `go test` + `golangci-lint` v2), the rules a change has to respect, and what the bar for a new module is. What must not break between 1.x releases is written down in [Compatibility](https://allan-nava.github.io/checkfleet/compatibility/).

Found a security issue? Don't open an issue — [report it privately](https://github.com/Allan-Nava/checkfleet/security/advisories/new). See [SECURITY.md](SECURITY.md).

## License

[PolyForm Noncommercial License 1.0.0](LICENSE) — source-available, free for **noncommercial** use (personal, research, education, nonprofits, government). Any commercial use requires a separate license — see [COMMERCIAL.md](COMMERCIAL.md). Releases published before v0.50.0 remain under the MIT license they shipped with.
