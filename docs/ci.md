---
title: CI integration
nav_order: 8
---

checkfleet is built to run in a pipeline. Because a check that ran is a
*success*, a normal run exits `0` regardless of findings — you decide when a
finding should fail the build.

## Gate with `--exit-on`

Pick the severity that should break the build:

```bash
checkfleet check all --config checkfleet.yml --exit-on bad
```

| `--exit-on` | Fails on | Use it when |
|---|---|---|
| *(unset)* | never | reporting only — the run is a success because it ran |
| `warn` | WARN, BAD, ERROR | you want the build red on the first sign of drift |
| `bad` | BAD, ERROR | the common choice: real problems, tolerating WARN |
| `error` | ERROR only | you only care that the checks could *measure* — a BAD target is someone else's alert |

`--exit-on-bad` is still accepted as an alias of `--exit-on bad`. If both are
given, the explicit `--exit-on` wins.

`ok` is rejected: it would fail every run, including all-green ones.

### A custom exit code

`--exit-code N` (1–125) changes the code a tripped gate returns, which is how
you let a wrapper script tell "checkfleet found something" apart from
"checkfleet itself broke":

```bash
checkfleet check all --config checkfleet.yml --exit-on bad --exit-code 42
```

The default stays `2`. Codes outside 1–125 are rejected: `0` would make the gate
a silent no-op, and 126+ collide with the shell's own "not executable" and
"killed by signal" range.

### Exit codes at a glance

| Code | Meaning |
|---|---|
| `0` | the run completed; no gate was set, or nothing reached the threshold |
| `2` (or `--exit-code`) | the gate tripped — findings at or above the threshold |
| `1` | **systemic failure**: unreadable config, unknown module, bad flag |

That last row is the important distinction. A gate that trips is a *result*; a
`1` means checkfleet could not do its job, and a pipeline that treats the two
the same will one day report a healthy fleet because the config file was
missing. Flag errors are caught before the run starts, so a typo in `--exit-on`
costs you a usage error rather than a full fleet sweep.

## GitHub Actions

Use the action:

```yaml
name: fleet-checks
on:
  schedule:
    - cron: "0 * * * *"   # hourly
  workflow_dispatch:

jobs:
  checks:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: Allan-Nava/checkfleet@v0.132.0
```

That is the whole job. Every input is optional and the defaults are the common
case: run `all` modules from `checkfleet.yml`, annotate the run, write the job
summary, fail on BAD/ERROR.

### Inputs

| Input | Default | |
|---|---|---|
| `version` | `latest` | release to install, e.g. `0.131.0` |
| `module` | `all` | `certs`, `http`, … |
| `config` | `checkfleet.yml` | path to the config |
| `stack` | — | comma-separated profiles, e.g. `region-eu,prod` |
| `output` | `github` | any sink list, e.g. `github,slack` |
| `out-file` | — | write the rendered output to a file |
| `exit-on` | `bad` | `warn`\|`bad`\|`error`; empty = report only |
| `exit-code` | `2` | code when the gate trips |
| `baseline` | — | baseline file (see below) |
| `fail-on-new` | `false` | gate only on new/worse findings |
| `min-severity` | — | drop findings below this severity |
| `target` | — | glob filter on targets |

It exposes one output, `exit-code`, so a workflow can react without re-running
anything:

```yaml
      - uses: Allan-Nava/checkfleet@v0.132.0
        id: fleet
        continue-on-error: true
        with:
          config: infra/checkfleet.yml
          exit-on: warn
      - if: steps.fleet.outputs.exit-code != '0'
        run: echo "findings reached the gate"
```

Pin it to a release tag rather than a branch. A moving `v1` alias tag is not
published yet, so use the exact version (`@v0.132.0`) — the action's own
`version` input is what controls which checkfleet binary it installs, and that
defaults to `latest` independently of the tag you pin the action to.

The action runs on **Linux and macOS** runners only; on Windows it fails with an
explicit message rather than something obscure.

### Without the action

The action is a convenience over one command, so nothing is lost by not using
it:

```yaml
      - uses: actions/setup-go@v5
        with:
          go-version: "1.25"
      - run: go install github.com/Allan-Nava/checkfleet/cmd/checkfleet@latest
      - run: checkfleet check all --config checkfleet.yml --output github --exit-on bad
```

That single command does three things:

1. **Annotations** — every WARN/BAD/ERROR finding is emitted as a workflow
   command (`::warning` / `::error`), so it shows up inline on the run and on
   the PR. OK findings are skipped on purpose: GitHub renders at most 10
   annotations per level per step, and spending that budget on green targets
   would push the real problems out of the view.
2. **Job summary** — the full Markdown report is written to
   `$GITHUB_STEP_SUMMARY` (appended, so it coexists with other steps).
3. **The gate** — `--exit-on bad` fails the job.

### Why not just pipe into `$GITHUB_STEP_SUMMARY`

Because this is silently broken:

```yaml
# DON'T: the gate never fires
run: checkfleet check all --config checkfleet.yml --output markdown --exit-on bad >> "$GITHUB_STEP_SUMMARY" | tee /dev/stderr
```

In a pipeline the shell reports the **last** command's status, so checkfleet's
exit code is replaced by `tee`'s `0` and the job stays green no matter what was
found. It only works with `set -o pipefail` (which the `run:` default shell does
*not* enable — you need an explicit `shell: bash`). `--output github` writes the
summary file itself, so there is no pipe and nothing to get wrong.

### Fanning out

`github` composes with the other sinks, since `--output` takes a list:

```bash
checkfleet check all --config checkfleet.yml --output github,slack --exit-on bad
```

## Code scanning (SARIF)

Upload the SARIF report and the findings become **Code scanning alerts** in the
Security tab, with history and dismissal, instead of scrolling back through run
logs:

```yaml
jobs:
  checks:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      security-events: write     # required to upload SARIF
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.25"
      - run: go install github.com/Allan-Nava/checkfleet/cmd/checkfleet@latest
      - name: Run checks
        # No gate here: let the upload happen, then gate in a later step if you
        # want the build red as well.
        run: checkfleet check all --config checkfleet.yml --output sarif --out-file checkfleet.sarif
      - uses: github/codeql-action/upload-sarif@v3
        with:
          sarif_file: checkfleet.sarif
```

Keep `--config` **repo-relative** (`checkfleet.yml`, not `/etc/checkfleet.yml`):
results are anchored to that path, and GitHub attaches each alert to the file it
names.

Alerts are matched across runs by a fingerprint of check + target that
deliberately excludes severity, so a certificate sliding from WARN to BAD
updates the existing alert rather than opening a second one.

## Adopting on a fleet that is already broken

A plain gate is unusable on a fleet with pre-existing problems: the first run is
red, it stays red, and within a week someone deletes the gate. A **baseline**
freezes the known debt so the build only fails on what appeared since.

```bash
# First run: records the current state and does NOT fail.
checkfleet check all --config checkfleet.yml --baseline checkfleet-baseline.json --fail-on-new

# Every run after: fails only on findings the baseline never saw, or worse ones.
checkfleet check all --config checkfleet.yml --baseline checkfleet-baseline.json --fail-on-new
```

Commit the baseline file next to the config. What counts as a failure:

| Baseline | Now | Gated? |
|---|---|---|
| BAD | BAD | no — known debt |
| BAD | WARN | no — it improved |
| WARN | BAD | **yes** — a regression on a known-imperfect target is still a regression |
| *(never seen)* | BAD | **yes** — new |
| OK | BAD | **yes** — new |
| BAD | *(gone)* | no |

`--fail-on-new` implies `--exit-on bad`; set `--exit-on` explicitly to gate at a
different severity. When the debt is genuinely paid down (or deliberately
accepted), re-record it:

```bash
checkfleet check all --config checkfleet.yml --baseline checkfleet-baseline.json --write-baseline
```

Two deliberate constraints: `--baseline` on its own never loosens an existing
gate — narrowing takes `--fail-on-new`, so adding a baseline to a pipeline can't
quietly disable its protection — and an unreadable or future-version baseline is
a systemic error (exit 1), not a silently empty one that would let everything
through.

## Gating on JSON

If you'd rather branch in a script, parse the `worst` field:

```bash
worst=$(checkfleet check all --config checkfleet.yml --output json | jq -r '.worst')
case "$worst" in
  BAD|ERROR) echo "fleet unhealthy: $worst"; exit 1 ;;
  *)         echo "fleet ok ($worst)" ;;
esac
```

## GitLab CI

No plugin needed — download the binary and run it. The JUnit sink turns the
findings into the pipeline's **Tests** tab, and the SARIF-equivalent for GitLab
is the JSON artifact.

```yaml
checkfleet:
  stage: test
  image: alpine:3
  variables:
    CHECKFLEET_VERSION: "0.131.0"
  before_script:
    - apk add --no-cache curl
    - curl -sSfL "https://github.com/Allan-Nava/checkfleet/releases/download/v${CHECKFLEET_VERSION}/checkfleet_${CHECKFLEET_VERSION}_linux_amd64.tar.gz" | tar -xz checkfleet
  script:
    - ./checkfleet check all --config checkfleet.yml --output junit --out-file report.xml --exit-on bad
  artifacts:
    when: always          # keep the report even when the gate fails the job
    reports:
      junit: report.xml
  rules:
    - if: $CI_PIPELINE_SOURCE == "schedule"
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
```

`when: always` matters: without it a tripped gate fails the job *and* discards
the report that explains why.

To schedule it, add a **pipeline schedule** in the project settings; the `rules`
above keep the job to scheduled runs and the default branch.

## TeamCity

A single command-line build step. Install (or download) checkfleet, run the
checks, and let `--exit-on bad` fail the build; emit a TeamCity service message
so the failure is readable in the build log.

```bash
#!/usr/bin/env bash
set -euo pipefail
go install github.com/Allan-Nava/checkfleet/cmd/checkfleet@latest
export PATH="$PATH:$(go env GOPATH)/bin"

# Human report into the build log.
checkfleet check all --config checkfleet.yml --output markdown

# Gate the build; surface a build problem on BAD/ERROR.
if ! checkfleet check all --config checkfleet.yml --exit-on bad; then
  echo "##teamcity[buildProblem description='checkfleet: fleet unhealthy (BAD/ERROR)']"
  exit 1
fi
```

Schedule it with a TeamCity **cron trigger** for periodic fleet checks, or wire
the [`serve`](usage.md#the-serve-command) exporter into your Prometheus and
alert there instead.

## Cron

```cron
# hourly, mail the report on BAD/ERROR only
0 * * * * checkfleet check all --config /etc/checkfleet.yml --exit-on bad --output markdown || mail -s "checkfleet: fleet unhealthy" ops@example.com
```
