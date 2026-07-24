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
| `--stack` | — | Overlay a per-stack profile `checkfleet.<stack>.yml` on the base config. See [Configuration → multi-stack](configuration.md#multi-stack-profiles). |
| `--output` | `text` | Output format: `text`, `markdown`, `json`, `junit`, `prometheus`, `slack`, or `webhook`. See [Output formats](output.md). |
| `--out-file` | — | Write the output atomically to this file instead of stdout (e.g. a node_exporter `.prom` file). |
| `--webhook-env` | `SLACK_WEBHOOK` | Env var holding the webhook URL (used by `--output slack` and `--output webhook`). |
| `--only` | — | Show only these checks (comma-separated, e.g. `--only certs,http`). |
| `--min-severity` | — | Show only findings at or above `ok`\|`warn`\|`bad`\|`error`. |
| `--target` | — | Show only targets matching this glob (e.g. `--target '*.example.com'`). |
| `--history` | — | JSONL file to append each run to; enables flap detection across runs. |
| `--flap-changes` | `3` | Minimum status changes in the window to flag flapping. |
| `--flap-window` | `10` | Number of recent runs to evaluate flapping over. |
| `--ping-url-env` | — | Env var with a dead-man's-switch URL (e.g. Healthchecks.io) to ping at the end of the run. |
| `--exit-on-bad` | off | Exit `2` when any BAD/ERROR finding is present. For CI gates. |

With `--history <file>`, each run is appended to a JSONL log and any
`check/target` that changed status at least `--flap-changes` times over the last
`--flap-window` runs gets an extra `flap` WARN finding — useful to spot
unstable targets that pass/fail intermittently. Zero dependencies (plain JSONL).

Filters apply to the rendered output (and therefore to `--exit-on-bad` and the
JSON `worst`), so `--min-severity bad --exit-on-bad` gates only on real
problems.

With `--ping-url-env`, checkfleet pings a dead-man's-switch at the end of the
run: the base URL on success, `<url>/fail` when the worst finding is BAD/ERROR.
Combined with cron it also catches the case where checkfleet didn't run at all.

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

## The `report-issues` command

Turn BAD/ERROR findings into tracker issues: one issue per `check/target`, opened
when it fails and **closed automatically when it recovers**. Idempotent — safe
to run on a schedule. Works with **GitHub** (via `gh`) or **GitLab** (via `glab`).

```bash
checkfleet report-issues --config checkfleet.yml               # GitHub (default)
checkfleet report-issues --config checkfleet.yml --forge gitlab
checkfleet report-issues --config checkfleet.yml --dry-run     # preview, no changes
```

`--forge github|gitlab` picks the tracker; the matching CLI (`gh`/`glab`) must be
installed and authenticated.

| Flag | Default | Meaning |
|---|---|---|
| `--config` | `checkfleet.yml` | Path to the YAML config. |
| `--stack` | — | Overlay a stack profile (see [multi-stack](configuration.md#multi-stack-profiles)). |
| `--dry-run` | off | Print what would open/close without touching any issue. |

Managed issues carry the `checkfleet-finding` label and a `[checkfleet] check/target`
title (the dedup key). Requires the [`gh`](https://cli.github.com/) CLI,
authenticated; in CI provide `GH_TOKEN`. GitLab isn't supported yet — the
tracker is behind an interface, so it can be added later.

## The `validate` command

Check the config without running any check — useful in CI or a pre-commit hook.
It reports missing targets/URLs/DSNs, incoherent thresholds (e.g. `warn` past
`crit`), and an empty `checks`. Exit `1` on any problem.

```bash
checkfleet validate --config checkfleet.yml            # exit 0 if usable
checkfleet validate --config checkfleet.yml --stack prod
```

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

## Explain a module

`checkfleet explain <module>` prints what a module checks and its key
thresholds; with no argument it lists all modules.

```bash
checkfleet explain            # list modules
checkfleet explain postgres   # what the postgres check verifies
```

## Shell completion

```bash
checkfleet completion bash > /etc/bash_completion.d/checkfleet   # bash
checkfleet completion zsh  > "${fpath[1]}/_checkfleet"           # zsh
checkfleet completion fish > ~/.config/fish/completions/checkfleet.fish
```

Completes subcommands, module names (after `check`/`explain`) and `--output`
formats.

## Live watch

`--watch <interval>` re-runs the checks on a timer and redraws a live terminal
view (text output), handy during an incident. Ctrl-C to stop.

```bash
checkfleet check all --config checkfleet.yml --watch 5s
```

## Diff vs the previous run

With `--history <file>`, `--diff` prints only what changed since the previous
recorded run — new / resolved / worsened / improved findings — instead of the
full table. Great for a cron that only reports deltas.

```bash
checkfleet check all --config checkfleet.yml --history runs.jsonl --diff
```
