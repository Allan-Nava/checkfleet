---
title: Output formats
nav_order: 6
description: >-
  How checkfleet renders findings — terminal text, an ops-style Markdown report
  you can paste into a runbook, JSON with a `worst` field for gating, and Slack
  Block Kit.
---

Pick one with `--output`. Every format renders the same findings, sorted
worst-first.

## Fan out to several sinks

`--output` takes a **comma-separated list**, so one run emits to several sinks at
once — the checks run only once:

```bash
# print JSON locally and push the report to Slack
checkfleet check all --config checkfleet.yml --output json,slack

# a file format plus two chat sinks
checkfleet check all --config checkfleet.yml --output markdown,slack,teams --out-file report.md
```

With multiple sinks each one is **isolated**: an unset env var or a down webhook
is reported on stderr but doesn't stop the other sinks or fail the run (the
[finding gate](ci.md) is separate). With a single `--output`, a sink error still
aborts the command as before. `--out-file` applies to the format renderer;
combining several *file* formats in one run isn't meaningful (the last wins).

## `text` (default)

For the terminal. One line per finding with a colored status glyph, then a
summary line.

```
🔴 BAD   http     https://example.com/health   HTTP 404 (want 200), 151ms
🟢 OK    certs    example.com:443              expires in 41 days (2026-09-02, CN=*.example.com)

2 checks: 1 OK, 0 WARN, 1 BAD, 0 ERROR (in 227ms)
```

The finding order is a de-facto API and applies to **every** output format:
findings are sorted **worst-first**, then by check, then by target, with a
**stable** sort (equal keys keep their original order). Exact-duplicate findings
(same check, target, status and message) are **de-duplicated**. Tools that parse
the output can rely on this ordering.

## `markdown`

An ops-style report you can paste into an incident doc or a PR: a summary, a
"Needs attention" section for the non-OK findings, and a full
table.

```bash
checkfleet check all --config checkfleet.yml --output markdown > report.md
```

## `json`

Machine-readable. Includes a top-level `worst` field with the worst status in
the run — the field to gate on in a pipeline.

```bash
checkfleet check all --config checkfleet.yml --output json | jq '.worst'
```

```json
{
  "worst": "BAD",
  "findings": [
    { "check": "http", "target": "https://example.com/health", "status": "BAD", "message": "HTTP 404 (want 200), 151ms" }
  ]
}
```

See [CI integration](ci.md) for using `worst` or `--exit-on-bad` to fail a build.

## `junit`

JUnit XML — one `<testcase>` per finding, a `<failure>` for BAD, an `<error>`
for ERROR, WARN kept passing (with a `<system-out>` note). Feed it to a CI test
tab (TeamCity, GitHub Actions test reporters).

```bash
checkfleet check all --config checkfleet.yml --output junit > report.xml
```

## `html`

A self-contained static HTML report (styles inlined, no external resources),
themed like this site: the worst-status pill, per-status count tiles, a
"Needs attention" section, and the full table. Nice to publish as a CI artifact
or attach to an incident.

```bash
checkfleet check all --config checkfleet.yml --output html --out-file report.html
```

## `github`

For GitHub Actions. Emits the findings as **workflow commands** on stdout, so
they become inline annotations on the run and on the PR, and appends the full
Markdown report to the job summary (`$GITHUB_STEP_SUMMARY`) when that variable
is set.

```bash
checkfleet check all --config checkfleet.yml --output github --exit-on bad
```

| Finding | Annotation |
|---|---|
| BAD, ERROR | `::error` |
| WARN | `::warning` |
| OK | *(none)* |

OK findings are skipped deliberately: GitHub shows at most 10 annotations per
level per step, so annotating green targets would hide the real ones. The
summary still lists everything.

Because the sink writes the summary file itself, you never pipe its output —
which is what makes the CI gate reliable, see
[CI integration](ci.md#why-not-just-pipe-into-github_step_summary). Outside
Actions (no `$GITHUB_STEP_SUMMARY`) it just prints the annotations, which is
handy for eyeballing the format locally.

## `sarif`

[SARIF 2.1.0](https://sarifweb.azurewebsites.net/), the interchange format for
static-analysis results. Upload it and the findings land in GitHub's **Code
scanning / Security** tab (and in any other SARIF-aware tool) — see
[CI integration](ci.md#code-scanning-sarif).

```bash
checkfleet check all --config checkfleet.yml --output sarif --out-file checkfleet.sarif
```

| Finding | SARIF level |
|---|---|
| BAD, ERROR | `error` |
| WARN | `warning` |
| OK | `none` |

Each **module** becomes a rule (`checkfleet/certs`, `checkfleet/http`, …) with
its description taken from `checkfleet explain`; each **finding** becomes a
result.

Three details that matter when reading the output:

- **BAD and ERROR share the level `error`** because SARIF has no third failure
  level. The engine's own status survives in `properties.status`, so a consumer
  can still tell "the target is unhealthy" from "the check could not measure".
- **Results are anchored to the config file**, line 1. SARIF is file-oriented
  and a checkfleet finding is about a network target, so there is no source line
  to blame; the config is the file that makes the target be checked at all. The
  real subject is in the message and in `properties.target`. Pass a
  repo-relative `--config` so the alerts attach to the file in the repo.
- **Fingerprints ignore severity** (`partialFingerprints` is built from
  check + target). A certificate going WARN → BAD stays *the same alert getting
  worse*, instead of appearing as a new one.

## `prometheus`

The Prometheus text-exposition format (same metrics as `serve`), for a one-shot
run instead of a scrape. With `--out-file` it's written atomically (temp +
rename), so it's safe to drop into the node_exporter **textfile collector**:

```bash
checkfleet check all --config checkfleet.yml --output prometheus \
  --out-file /var/lib/node_exporter/textfile/checkfleet.prom
```

`--out-file` works for any printable format (writes to the file instead of
stdout).

## `csv`

Emits CSV with a header row — `status,check,target,message` — worst first, one
finding per row. Fields are quoted/escaped (commas and newlines in messages are
safe), for spreadsheets or ingestion into another system.

```bash
checkfleet check all --config checkfleet.yml --output csv --out-file findings.csv
```

## `slack`

Posts a [Block Kit](https://api.slack.com/block-kit) message to a Slack incoming
webhook instead of printing: a header, the summary line, then the non-OK
findings (worst first, capped). The webhook URL is read from an environment
variable — never passed on the command line or stored in config.

```bash
export SLACK_WEBHOOK="https://hooks.slack.com/services/…"
checkfleet check all --config checkfleet.yml --output slack
# or point at a different env var:
checkfleet check all --config checkfleet.yml --output slack --webhook-env SLACK_WEBHOOK_OPS
```

If the env var is empty the command errors (nothing is sent). A run that posts
successfully prints `report sent to Slack`.

## `discord` / `teams`

Post to a **Discord** webhook (a rich embed) or a **Microsoft Teams** incoming
webhook (a MessageCard) — the summary plus the non-OK findings (worst first,
capped), colored by the worst status. Same model as `slack`: the URL comes from
`--webhook-env`, never the command line.

```bash
export DISCORD_WEBHOOK="https://discord.com/api/webhooks/…"
checkfleet check all --config checkfleet.yml --output discord --webhook-env DISCORD_WEBHOOK

export TEAMS_WEBHOOK="https://outlook.office.com/webhook/…"
checkfleet check all --config checkfleet.yml --output teams --webhook-env TEAMS_WEBHOOK
```

## `telegram`

Sends a plain-text message via the **Telegram Bot API** (`sendMessage`): the
summary line then the non-OK findings (worst first, capped, within the 4096-char
limit). The bot token and chat id come from the environment — never the command
line — via `--telegram-token-env` (default `TELEGRAM_TOKEN`) and
`--telegram-chat-env` (default `TELEGRAM_CHAT_ID`).

```bash
export TELEGRAM_TOKEN="123456:ABC-DEF…"
export TELEGRAM_CHAT_ID="-1001234567890"
checkfleet check all --config checkfleet.yml --output telegram
```

## `webhook`

POSTs the JSON output to a generic webhook (URL from `--webhook-env`), for any
system that ingests JSON — a Teams/Discord relay, a custom collector, etc.

```bash
export MY_HOOK="https://hooks.example/checkfleet"
checkfleet check all --config checkfleet.yml --output webhook --webhook-env MY_HOOK
```

Pass `--template FILE` to shape the payload with a Go
[text/template](https://pkg.go.dev/text/template) instead of the default JSON.
The template is executed against `{{.Title}}`, `{{.Worst}}`, `{{.Total}}`,
`{{.OK}}`/`{{.WARN}}`/`{{.BAD}}`/`{{.ERROR}}`, and `{{range .Findings}}` (each
with `.Check`, `.Target`, `.Status`, `.Message`). Unknown fields are an error, so
typos don't ship silently.

```bash
checkfleet check all --config checkfleet.yml --output webhook \
  --webhook-env MY_HOOK --template payload.tmpl
```


## `otlp`

An OTLP/HTTP **metrics** request in JSON encoding — the same gauges as
`prometheus` (`checkfleet.finding.status`, `.findings.total`, `.worst.status`),
hand-built with **zero dependencies** (no OpenTelemetry SDK). POST it to a
collector's `/v1/metrics`:

```bash
checkfleet check all --config checkfleet.yml --output otlp \
  | curl -s --data-binary @- -H 'content-type: application/json' \
      http://otel-collector:4318/v1/metrics
```
