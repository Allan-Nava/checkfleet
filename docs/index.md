---
title: Home
nav_order: 1
---

**A fleet of infrastructure checks in one binary.**

checkfleet runs *domain-aware* health checks — the kind generic monitoring can't
express — and reports them as terminal output, an ops-style markdown report, or
JSON. One static Go binary, one YAML config, no agents, no server.

```
$ checkfleet check all --config checkfleet.yml

🔴 BAD   http     https://example.com/health   HTTP 404 (atteso 200), 151ms
🟢 OK    certs    example.com:443              scade tra 41 giorni (2026-09-02, CN=*.example.com)
🟢 OK    http     https://example.com/         HTTP 200, 168ms

3 check: 2 OK, 0 WARN, 1 BAD, 0 ERROR (in 227ms)
```

## Documentation

- [Installation](installation.md) — install from source or grab a release binary
- [Configuration](configuration.md) — the `checkfleet.yml` reference
- [Usage](usage.md) — commands, flags, output formats, exit codes
- [Modules](modules.md) — what each check knows how to verify
- [Output formats](output.md) — text, markdown, JSON
- [CI integration](ci.md) — gating a pipeline on findings
- [Development](development.md) — adding a module

## Philosophy

Don't rebuild Prometheus or Grafana. checkfleet fills the layer they can't:
checks that need **domain knowledge** (what "healthy" means for a TLS estate, a
NATS cluster, an HLS stream), runnable from CI, cron, or your laptop, with
reports you can paste straight into your ops docs.

- **Exit code 0 even on WARN/BAD findings** — a check that ran *is* a success.
  Gate on the output, or use `--exit-on-bad` for CI.
- **Worst findings first** — the thing you must look at is the first line.
- **Fleet-aware** — point the certs check at your Ansible inventory and every
  host becomes a target.

The roadmap of upcoming modules lives in
[BACKLOG.md](https://github.com/Allan-Nava/checkfleet/blob/main/BACKLOG.md).
