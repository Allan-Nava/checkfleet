---
title: Agents
nav_order: 13
description: >-
  Install the checkfleet agent skill and use the CLI correctly from an AI
  assistant — the two semantics that decide whether the output is read right,
  and how to call it as an MCP server over stdio or HTTP.
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

checkfleet also ships an MCP server, a thin adapter over the same config,
registry and runner the CLI uses:

```bash
checkfleet mcp --config checkfleet.yml
```

The transport is the only thing that changes: the findings, the status model
and the exit-code-free "a check that ran is a success" semantics are the CLI's.
The single binary stays the canonical way to inspect a fleet.

### Tools

| Tool | Arguments | Mirrors |
|---|---|---|
| `checkfleet_run` | `module` (default `all`) | `checkfleet check` — findings worst-first, with `status`, `count`, `runbook`, `remediation` |
| `checkfleet_list_modules` | — | every known module, and the ones configured |
| `checkfleet_validate` | — | `checkfleet validate` — `valid`, `problems` with did-you-mean `suggestion`, `advisory` notes that do not invalidate, `load_error` when the file cannot load |
| `checkfleet_explain` | `module` (omit to list all) | `checkfleet explain` + `checkfleet perms` — thresholds, symptoms, and the least privilege the module needs |
| `checkfleet_targets` | `module`, `discover` (default `true`) | `checkfleet targets --output json` — hostnames only, never a DSN; discovery failures come back as `discovery_errors`, not as a failed call |

A tool that fails (unknown module, unreadable config) returns a normal result
with `isError: true`, so the model reads the reason and corrects the call. An
unknown tool or method is a JSON-RPC protocol error.

### stdio transport

- the default, and the right choice for a local desktop agent
- one JSON-RPC 2.0 message per line in, one response per line out
- a malformed line is answered with a parse error (`-32700`), it does not end
  the session; notifications get no reply

### HTTP transport

For an orchestrator that cannot spawn a local process:

```bash
export CHECKFLEET_MCP_TOKEN=...   # from your secret store, never in a flag or the config
checkfleet mcp --config checkfleet.yml --listen 127.0.0.1:8765
```

- one endpoint, `POST /mcp`: MCP *Streamable HTTP* in its stateless form —
  each POST carries one message and gets its response as `application/json`
- `GET /mcp` answers `405`: the server never pushes messages, so there is no
  SSE stream and no session id — every request stands alone, like a CLI run
- **authentication is mandatory**: `Authorization: Bearer $CHECKFLEET_MCP_TOKEN`,
  compared in constant time; the server refuses to start without the variable
- **Origin check** against DNS rebinding: a request carrying an `Origin` header
  is rejected with `403` unless the origin is listed in `--allow-origin`
  (comma-separated); requests without `Origin` (non-browser clients) pass
- bodies over 1 MiB are rejected with `413`
- bind to loopback, or put it behind your own TLS-terminating proxy: the
  server speaks plain HTTP

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

- the config file is the authority: the server does not invent or fetch state,
  and no tool writes anything
- no state is kept between calls
- no network listener unless `--listen` is given, and then never without a token
- `checkfleet_targets` returns hostnames extracted from DSNs/URIs, never the
  values themselves, so credentials in a connection string do not reach the model
- the agent must still treat the CLI as the source of truth for operator
  actions; MCP is a tool adapter for machine-driven orchestration
