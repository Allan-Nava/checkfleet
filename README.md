# checkfleet

**A fleet of infrastructure checks in one binary.**

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
# or download a binary from the GitHub releases
```

## Modules

| Module | What it checks |
|---|---|
| `certs` | TLS certificate expiry (WARN/BAD thresholds in days) for explicit targets **and/or every host of an Ansible INI inventory** |
| `http` | HTTP probes: expected status, max latency (WARN), body substring |

More on the way (see [BACKLOG.md](BACKLOG.md)): `nats`, `haproxy`, `stream` (HLS/DASH), `patroni`, Slack output, Prometheus exporter mode.

## Configuration

```yaml
# checkfleet.yml
timeout_seconds: 30
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

## Usage

```bash
checkfleet check all   --config checkfleet.yml                    # terminal
checkfleet check certs --config checkfleet.yml --output markdown  # ops report
checkfleet check http  --config checkfleet.yml --output json      # machine-readable (includes "worst")
checkfleet check all   --config checkfleet.yml --exit-on-bad      # exit 2 on BAD/ERROR, for CI gates
```

Finding statuses: `OK`, `WARN` (threshold crossed), `BAD` (target unhealthy), `ERROR` (the check itself could not measure — network, handshake).

## Development

```bash
go test ./...    # all tests run against local in-test servers — no network needed
go vet ./...
go build -o checkfleet ./cmd/checkfleet
```

Adding a module: implement `engine.Check` in `internal/checks/<name>`, add its typed config in `internal/engine/config.go`, wire it in `cmd/checkfleet/main.go`, and test it against a local fixture server.

## License

MIT
