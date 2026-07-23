---
title: Output formats
---

[← back to index](index.md)

# Output formats

Pick one with `--output`. Every format renders the same findings, sorted
worst-first.

## `text` (default)

For the terminal. One line per finding with a colored status glyph, then a
summary line.

```
🔴 BAD   http     https://example.com/health   HTTP 404 (atteso 200), 151ms
🟢 OK    certs    example.com:443              scade tra 41 giorni (2026-09-02, CN=*.example.com)

2 check: 1 OK, 0 WARN, 1 BAD, 0 ERROR (in 227ms)
```

The finding order is a de-facto API: worst-first, stable per check/target. Tools
that parse the text output can rely on it.

## `markdown`

An ops-style report you can paste into an incident doc or a PR: a summary, a
"Da guardare" (things to look at) section for the non-OK findings, and a full
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
    { "check": "http", "target": "https://example.com/health", "status": "BAD", "message": "HTTP 404 (atteso 200), 151ms" }
  ]
}
```

See [CI integration](ci.md) for using `worst` or `--exit-on-bad` to fail a build.
