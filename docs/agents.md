---
title: Agents
nav_order: 13
description: >-
  Install the checkfleet agent skill and use the CLI correctly from an AI
  assistant — the two semantics that decide whether the output is read right,
  and why there is no MCP server.
---

# Using checkfleet from an AI assistant

checkfleet ships an **agent skill**: a short document that teaches an assistant
what the tool does, which commands exist, and — the part that actually matters —
how to read the output without drawing the wrong conclusion.

## Install

The skill lives inside the binary, so it is always the version that matches the
`checkfleet` you are running:

```bash
checkfleet skill install            # → ~/.claude/skills/checkfleet/
checkfleet skill install --dir .    # → ./checkfleet/, for a project-local install
checkfleet skill print              # → stdout, for your own installer
```

Install it **globally**, not inside a repo. checkfleet is a tool you point *at*
your infrastructure from wherever you happen to be working; a per-repo copy goes
stale the moment you upgrade the binary.

Re-run `checkfleet skill install` after an upgrade. It overwrites, and it is
idempotent.

## What the skill contains

`SKILL.md` is deliberately small — under 6 KB, enforced by a test — because it
is always in context and context is the scarce resource. It carries the two
rules that decide whether an assistant reads results correctly:

**Exit code 0 does not mean healthy.** A check that ran is a success even when
it found something broken. Gating requires `--exit-on bad`. An assistant that
infers health from the exit code reports the opposite of the truth.

**`ERROR` is not `BAD`.** `BAD` means the target is unhealthy; `ERROR` means the
check could not measure. Reading "the database is down" from an `ERROR` is a
claim the data does not support — the honest reading is "we could not tell".

It also points at `--output json` and the `worst` field instead of grepping the
text renderer, whose wording the
[compatibility contract]({{ '/compatibility' | relative_url }}) explicitly does
*not* freeze.

Two references load on demand: `references/modules.md` (every module and what it
detects) and `references/config-schema.md` (keys, types, defaults).

## How it stays true

A skill that confidently cites a flag which no longer exists is worse than no
skill at all: the assistant keeps trying it and blames the environment. Three
gates keep that from happening.

- The references are **generated** from `internal/registry` and the config
  structs by `go run ./cmd/gen-skill` — defaults included, read by applying the
  real defaults rather than copied out of comments.
- CI **regenerates them and fails if the diff is not empty**, so a new module
  cannot land while the skill still lists the old set.
- A test compiles the binary and asserts that **every command and flag the skill
  shows as runnable exists in its usage**.

## Why not an MCP server

Not now. MCP would be the right shape if checkfleet needed to hold state across
calls or stream results — it does not. It is a single binary that takes a config
and prints a document, which a shell tool already exposes perfectly well, and
every assistant can run a shell command while MCP support varies. A server would
add a process to supervise, a transport to debug and a second surface to keep
compatible, in exchange for nothing the CLI does not already give.

If that changes — long-running fleet state, subscriptions to status transitions
— it gets reconsidered on the merits.
