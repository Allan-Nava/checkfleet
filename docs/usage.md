---
title: Usage
nav_order: 4
---

```
checkfleet check <all|certs|http|nats|haproxy|stream|patroni|consul|postgres|dns> --config checkfleet.yml [--output text|markdown|json] [--exit-on-bad]
checkfleet version
```

## The `check` command

```bash
checkfleet check all   --config checkfleet.yml                    # every configured module
checkfleet check certs --config checkfleet.yml                    # a single module
checkfleet check http  --config checkfleet.yml --output json      # machine-readable
```

`all` runs every module present in the config. Naming a single module runs only
that one; if it isn't configured, the command fails.

## Flags

| Flag | Default | Meaning |
|---|---|---|
| `--config` | `checkfleet.yml` | Path to the YAML config. |
| `--output` | `text` | Output format: `text`, `markdown`, `json`, or `slack`. See [Output formats](output.md). |
| `--webhook-env` | `SLACK_WEBHOOK` | Env var holding the Slack webhook URL (used by `--output slack`). |
| `--exit-on-bad` | off | Exit `2` when any BAD/ERROR finding is present. For CI gates. |

## The `serve` command

Run checkfleet as a Prometheus exporter: it re-runs the configured checks on an
interval and exposes the latest findings as metrics on `/metrics`.

```bash
checkfleet serve --config checkfleet.yml --listen :9876 --interval 60s
```

| Flag | Default | Meaning |
|---|---|---|
| `--config` | `checkfleet.yml` | Path to the YAML config. |
| `--listen` | `:9876` | Address to listen on. |
| `--interval` | `60s` | How often to re-run the checks. |

Metrics exposed:

| Metric | Meaning |
|---|---|
| `checkfleet_finding_status{check,target}` | Severity of each finding: `0`=OK, `1`=WARN, `2`=BAD, `3`=ERROR (worst wins per check/target). |
| `checkfleet_findings_total{status}` | Count of findings per status. |
| `checkfleet_worst_status` | Worst severity across the run. |
| `checkfleet_run_duration_seconds` | Duration of the last run. |
| `checkfleet_last_run_timestamp_seconds` | Unix time of the last run. |

This is the bridge to Grafana/alerting: checkfleet keeps the domain logic, and
Prometheus does the graphing and alerting — it doesn't replace them.

## Finding statuses

| Status | Meaning |
|---|---|
| `OK` | Healthy. |
| `WARN` | A soft threshold was crossed (e.g. cert near expiry, slow response). |
| `BAD` | The target is unhealthy (e.g. cert expired, wrong HTTP status). |
| `ERROR` | The check itself **could not measure** — network failure, TLS handshake error. Not the same as BAD. |

Findings are always sorted **worst-first**, stable per check/target — the first
line is the thing you must look at.

## Exit codes

checkfleet distinguishes "a check found a problem" from "checkfleet itself
failed". A check that ran *is* a success.

| Code | When |
|---|---|
| `0` | The run completed — **even with WARN/BAD/ERROR findings**, unless `--exit-on-bad` is set. |
| `2` | `--exit-on-bad` was set **and** at least one BAD/ERROR finding is present. |
| `64` | Usage error (missing/unknown subcommand). |
| `1` | Systemic error: unreadable config, unknown module, unknown output format. |

This semantics is intentional and stable — see [CI integration](ci.md) for how
to gate on it.
