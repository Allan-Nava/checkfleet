<p align="center">
  <img src="docs/assets/logo.png" alt="checkfleet logo" width="116" height="116">
</p>

<h1 align="center">checkfleet</h1>

<p align="center"><strong>A fleet of <em>domain-aware</em> infrastructure checks in one Go binary.</strong></p>

<p align="center">
  <a href="https://github.com/Allan-Nava/checkfleet/releases"><img alt="Latest release" src="https://img.shields.io/github/v/tag/Allan-Nava/checkfleet?label=release&sort=semver&color=10b981"></a>
  <a href="https://github.com/Allan-Nava/checkfleet/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/Allan-Nava/checkfleet/actions/workflows/ci.yml/badge.svg"></a>
  <a href="LICENSE"><img alt="License: MIT" src="https://img.shields.io/badge/license-MIT-10b981"></a>
  <img alt="Go" src="https://img.shields.io/github/go-mod/go-version/Allan-Nava/checkfleet?color=10b981">
</p>

<p align="center">📖 <strong>Full documentation: <a href="https://allan-nava.github.io/checkfleet/">allan-nava.github.io/checkfleet</a></strong></p>

---

checkfleet runs *domain-aware* health checks — the kind that generic monitoring can't express — and reports them as terminal output, an ops-style markdown report, or JSON. One static Go binary, one YAML config, no agents, no server.

```
$ checkfleet check all --config checkfleet.yml

🔴 BAD   http     https://example.com/health   HTTP 404 (atteso 200), 151ms
🟢 OK    certs    example.com:443              scade tra 41 giorni (2026-09-02, CN=*.example.com)
🟢 OK    http     https://example.com/         HTTP 200, 168ms

3 check: 2 OK, 0 WARN, 1 BAD, 0 ERROR (in 227ms)
```

## Philosophy

Don't rebuild Prometheus or Grafana. checkfleet fills the layer they can't: checks that need **domain knowledge** (what "healthy" means for a TLS estate, a NATS cluster, an HLS stream), runnable from CI, cron, or your laptop, with reports you can paste straight into your ops docs.

- **Exit code 0 even on WARN/BAD findings** — a check that ran *is* a success. Gate on the output, or use `--exit-on-bad` for CI.
- **Worst findings first** — the thing you must look at is the first line.
- **Fleet-aware** — point the certs check at your Ansible inventory and every host becomes a target.

## Install

```bash
go install github.com/Allan-Nava/checkfleet/cmd/checkfleet@latest
# or: brew install Allan-Nava/tap/checkfleet   (once the tap is published)
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

More modules on the roadmap (see [BACKLOG.md](BACKLOG.md)): `ldap`, `kafka`, `mongodb`, plus more alerting outputs.

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
checkfleet check all   --config checkfleet.yml                    # terminal
checkfleet check certs --config checkfleet.yml --output markdown  # ops report
checkfleet check nats  --config checkfleet.yml --output markdown  # NATS cluster health
checkfleet check postgres --config checkfleet.yml                 # PostgreSQL (read-only SQL)
checkfleet check dns   --config checkfleet.yml                    # DNS resolution & drift
checkfleet check http  --config checkfleet.yml --output json      # machine-readable (includes "worst")
checkfleet check all   --config checkfleet.yml --exit-on-bad      # exit 2 on BAD/ERROR, for CI gates
checkfleet check all   --config checkfleet.yml --output slack     # post a Block Kit report to a Slack webhook

# scope the findings
checkfleet check all --config checkfleet.yml --only certs,http    # run/report only these checks
checkfleet check all --config checkfleet.yml --min-severity warn  # hide OK
checkfleet check all --config checkfleet.yml --target 'example.*'  # glob on target

# other commands
checkfleet validate     --config checkfleet.yml                   # validate the config without running checks
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

## License

MIT
