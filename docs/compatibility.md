---
title: Compatibility
nav_order: 12
description: >-
  What checkfleet promises not to break — the config schema, the JSON output,
  exit codes, finding identity, the history file and the Prometheus metric
  names — and the deprecation policy for changing any of them.
keywords: checkfleet compatibility, semantic versioning, stable output schema, deprecation policy
---

# Compatibility

checkfleet gets embedded in things that outlive a release: a config file in a
repo, a `jq` expression in a pipeline, a Grafana panel, a cron job that has been
green for a year. This page says exactly which of those are safe to depend on.

The short version: **anything on this page is stable for the whole 1.x line.**
If it has to change, it changes through the deprecation policy below — never
silently, never in a patch.

## Versioning

checkfleet follows [semantic versioning](https://semver.org/) on the surfaces
listed here.

| Change | Version bump |
|---|---|
| A stable surface changes meaning or disappears | major |
| A new module, output, flag or field | minor |
| A fix that leaves every stable surface intact | patch |

Adding a field to an output is a **minor**, not a major: consumers are expected
to ignore fields they do not know. Removing or repurposing one is a major.

{: .note }
> Before 1.0 every release was tagged as a minor or patch regardless, because no
> stability was promised yet. The guarantees on this page start at **1.0.0**.

### Getting to 1.0

The 1.0 covers the **CLI** — the binary, its config schema, its outputs and its
exit codes. The [desktop app](desktop.md) is deliberately not in scope and stays
beta with its own version.

It ships through a release candidate rather than straight from a `0.x` tag:
`v1.0.0-rc.1` first, then a stretch of real use on a real fleet, then `v1.0.0`.
The reason is not ceremony. Everything on this page is a promise that outlives
the release that makes it, and a schema nobody has run in anger is exactly the
kind of thing you discover you got wrong the week after you promised not to
change it. If the rc turns something up, it changes in the next rc — that is what
the rc is for.

## What is stable

### 1. The config schema

Every documented key in [Configuration](configuration.md) — its name, its type,
its default, and what it does. A config that works on 1.0 works on every later
1.x without edits.

Two properties are part of the contract, not accidents:

- **Unknown keys do not abort the run and do not change the exit code.** A config
  written for a newer checkfleet runs on an older one, ignoring what it does not
  know — that tolerance is the point, and it is why `KnownFields` is deliberately
  *not* enabled. But tolerated is not the same as hidden: every command that acts
  on a config prints a notice on **stderr** naming the ignored key and the
  closest valid name, and [`validate`](usage.md) and `doctor` report them as
  problems. An ignored key means the module never runs, so the run would
  otherwise report a healthy fleet having checked nothing.
- **`${VAR}` interpolation** (`${VAR}`, `${VAR:-default}`, `${file:/path}`,
  `$${` for a literal) is applied to config values before parsing.

### 2. The JSON output

`--output json` emits a document with a top-level `schema` field. These keys are
stable:

| Key | Type | Meaning |
|---|---|---|
| `schema` | number | JSON document format version (currently `1`) |
| `findings` | array | every finding, in the documented order |
| `summary` | object | count per status |
| `worst` | string | worst status across the findings — **this is the field to gate on** |
| `started` | string | RFC 3339 start time of the run |
| `duration_ns` | number | run duration in nanoseconds |
| `labels` | object | global labels from the config, omitted when none are set |

And within a finding:

| Key | Type | Meaning |
|---|---|---|
| `check` | string | module name |
| `target` | string | what was checked |
| `status` | string | `OK`, `WARN`, `BAD` or `ERROR` |
| `message` | string | human-readable detail — **not** stable text, see below |
| `value` | number | optional scalar metric, omitted when the check has none |
| `unit` | string | unit of `value` (`ms`, `s`, `days`, `bytes`, …), omitted with it |

`message` is stable as a *field*, not as *text*. Finding messages get clearer
between releases; matching on their wording is the thing this page cannot
protect. Gate on `worst` or on `status`, never on a substring of `message`.

### 3. Exit codes

| Code | Meaning |
|---|---|
| `0` | the run completed — no gate was set, or nothing reached the threshold |
| `2` (or `--exit-code N`) | the gate tripped |
| `1` | systemic failure: unreadable config, unknown module, bad flag |

The load-bearing rule: **findings do not fail the run by themselves.** A check
that ran and found a problem is a *successful* check — you decide with
`--exit-on` when a finding should break the build. A `1` means checkfleet could
not do its job. See [CI & pipelines](ci.md).

Diagnostic commands (`validate`, `doctor`, `targets`, `explain`, `init`,
`completion`, `version`) never gate: they exit `0` unless something systemic
went wrong. `validate` is the exception that exits `1` on a config problem —
that *is* its job.

### 4. Status semantics

`OK` → `WARN` → `BAD` → `ERROR`, in that severity order, and `ERROR` means
**checkfleet could not measure** (network, handshake, timeout) — not "the target
is unhealthy". A monitor that treats `ERROR` as `BAD` pages someone about a
firewall rule as if the database were down. This distinction will not change.

### 5. Finding order

Findings are sorted **worst-first, then by check, then by target**, and the sort
is stable. Anyone parsing the text output depends on it, so it is part of the
contract. Identical findings (same check, target and status) are deduplicated.

### 6. Finding identity

A finding is identified by the pair **`check` + `target`**. This is not just
cosmetic: it is the deduplication key for `report-issues`, for `alert`, for the
history file and for `--baseline`. Renaming a module or changing how a module
spells its target makes every open issue reopen and every resolved alert
re-fire, so within 1.x a module keeps its name and its target format.

### 7. The history and baseline files

`--history file` is append-only JSONL, one record per run, with a `sv` schema
version on every line (currently `1`). Keys are short because the file grows
forever:

| Record | Key | Meaning |
|---|---|---|
| | `t` | run timestamp, Unix seconds |
| | `sv` | schema version of the record |
| | `f` | the run's entries |
| Entry | `c` | check |
| | `g` | target |
| | `s` | status |
| | `v` | optional numeric value |
| | `u` | unit of `v` |

Reading is backward and forward compatible by construction: a record written
before `sv` existed reads as version 1, and a record stamped with a *newer*
version is skipped and reported rather than misread — a wrong diff is worse than
a loud failure.

`--baseline file` is a JSON document with its own `version` field, and a
baseline whose version this binary does not understand is rejected outright with
the command to re-record it.

### 8. Prometheus metric names

Metric and label names are stable — renaming one breaks a dashboard silently,
because the query simply returns no data.

| Metric | Labels | Meaning |
|---|---|---|
| `checkfleet_finding_status` | `check`, `target` + global labels | severity per finding (`0`=OK, `1`=WARN, `2`=BAD, `3`=ERROR) |
| `checkfleet_findings_total` | `status` + global labels | count per status |
| `checkfleet_worst_status` | global labels | worst severity across the fleet |
| `checkfleet_run_duration_seconds` | global labels | duration of the last run |
| `checkfleet_last_run_timestamp_seconds` | global labels | when the last run finished |
| `checkfleet_module_findings` | `module` | findings produced per module (`serve`) |
| `checkfleet_module_errors` | `module` | ERROR findings per module (`serve`) |

The severity encoding (`0`/`1`/`2`/`3`) is part of the contract too: alert rules
compare against those numbers.

### 9. Command and flag names

Documented subcommands and flags keep working. A flag can gain a new value (as
`--exit-on-bad` became an alias of `--exit-on bad`) but does not change meaning.

## What is *not* stable

Depending on any of these is fine — just expect it to move in a minor release:

- **Finding message wording.** Improved freely. Never match on it.
- **The Go packages.** Everything lives under `internal/`, which the Go
  toolchain itself forbids importing. checkfleet is a tool, not a library.
- **Text and Markdown layout.** Written for humans; columns and section titles
  get rearranged. Use `json`, `csv` or `junit` for machines.
- **The desktop app.** Deliberately **outside** the 1.0: it carries its own
  version, is labelled **beta**, and its views, bindings and stored preferences
  can change in any release. It is also not code-signed on macOS yet. Promising
  stability for an unsigned app whose window is still being redesigned would be a
  promise made to be broken; the CLI is the source of truth for behaviour. See
  [Desktop app](desktop.md).
- **`serve` HTML pages** (`/healthz` and `/readyz` bodies are stable; the human
  page is not).
- **The exact text of errors and warnings on stderr.**

## Deprecation policy

When something stable has to change:

1. **The old form keeps working** for the rest of the 1.x line. No exceptions,
   including config keys — a config that stops loading after an upgrade is the
   failure mode this policy exists to prevent.
2. **It warns.** Using a deprecated form prints a notice on stderr naming the
   replacement. Warnings go to stderr, never into the rendered output, so they
   cannot corrupt a parsed document or a webhook payload.
3. **It is documented** in the release notes for that version and marked
   *deprecated* here, with what replaces it.
4. **It is removed only in the next major**, never in a minor or a patch.

New behaviour that would change existing results arrives **opt-in**, behind a
flag or a config key that defaults to today's behaviour.

## Reporting a compatibility break

If an upgrade broke something on this page, that is a bug, not a policy
decision — open an issue with the two versions and the config or command
involved. Include the output of `checkfleet version`.

## How this page stays true

It is enforced by tests, not by good intentions. The keys, metric names and
schema versions listed here are asserted in
`internal/output/contract_test.go` and `internal/history/contract_test.go`, and
those tests also check that everything the renderers emit **appears on this
page** — so a format change cannot land without this document being updated in
the same commit.
