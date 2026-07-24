---
title: Home
nav_order: 1
layout: home
description: A fleet of domain-aware infrastructure checks in one Go binary.
---

## Quickstart

```bash
# build from source
go build -o checkfleet ./cmd/checkfleet

# run every configured check
./checkfleet check all --config checkfleet.yml

# just TLS expiry, as a Markdown report
./checkfleet check certs --config checkfleet.yml --output markdown
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
