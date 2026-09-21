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

`checkfleet perms` answers the access question directly, and the skill points at
the same data, so an assistant asked "what does this need on my database?"
replies with the grant instead of guessing.

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

## MCP server

checkfleet also exposes a minimal stdio MCP server for automation-friendly
clients:

```bash
checkfleet mcp --config checkfleet.yml
```

It speaks JSON-RPC over stdin/stdout and exposes tools such as
`checkfleet_run`, `checkfleet_list_modules` and `checkfleet_validate`. The tool
surface is intentionally small: it asks the same config and runner that the CLI
uses, and it returns the same findings structure, so the transport is the only
thing that changes.

This is the right shape when an assistant or orchestrator wants to call the
runner as a tool without shelling out, but it does not replace the CLI: the
single-binary command remains the canonical way to inspect a fleet, and the MCP
server is a thin adapter over the same engine.

## MCP technical contract

The server is intentionally small and transport-agnostic. It is a JSON-RPC
wrapper over the same runner used by the CLI, so a client gets the same
configuration semantics, status model and filtering rules without a second
implementation of the check engine.

### Transport

- stdio is the default and safest option for local desktop agents
- stdin/stdout is line-delimited JSON and uses the standard JSON-RPC 2.0 envelope
- each request includes `jsonrpc`, `id`, `method`, and `params`
- the server replies with `jsonrpc`, `id` and `result` or `error`

### Tools

The minimal contract is:

- `tools/list` → declarations for available tools
- `tools/call` → execution of a tool and structured output
- `checkfleet_run` → run one or more configured modules with the active config
- `checkfleet_list_modules` → enumerate configured and available modules
- `checkfleet_validate` → validate the YAML and return machine-readable issues

A larger follow-up can add `checkfleet_explain` and `checkfleet_targets`, but
those should still be thin adapters over `internal/moduledoc` and the registry,
not custom logic.

### Claude Desktop configuration

```json
{
  "mcpServers": {
    "checkfleet": {
      "command": "/usr/local/bin/checkfleet",
      "args": ["mcp", "--config", "/path/to/checkfleet.yml"],
      "env": {
        "CHECKFLEET_NO_COLOR": "1"
      }
    }
  }
}
```

### VS Code configuration

```json
{
  "servers": {
    "checkfleet": {
      "type": "stdio",
      "command": "/usr/local/bin/checkfleet",
      "args": ["mcp", "--config", "/path/to/checkfleet.yml"]
    }
  }
}
```

### Security assumptions

- the config file is the authority: the tool does not invent or fetch state
- no long-lived server state is required for the first version
- no remote network listener by default; any HTTP/SSE variant must sit behind
  explicit auth and a separate trust boundary
- the agent must still use the CLI as the source of truth for operator actions,
  while MCP is only a tool adapter for machine-driven orchestration

This makes the server easy to reason about, easy to test, and consistent with
checkfleet’s rule that the CLI remains the canonical operational interface.
