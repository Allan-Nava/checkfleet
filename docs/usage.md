---
title: Usage
nav_order: 4
---

[← back to index](index.md)

# Usage

```
checkfleet check <all|certs|http> --config checkfleet.yml [--output text|markdown|json] [--exit-on-bad]
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
| `--output` | `text` | Output format: `text`, `markdown`, or `json`. See [Output formats](output.md). |
| `--exit-on-bad` | off | Exit `2` when any BAD/ERROR finding is present. For CI gates. |

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
