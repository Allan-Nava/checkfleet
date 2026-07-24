---
title: Development
nav_order: 8
---

```bash
go test ./...    # unit tests + modules against local in-test servers — no network
go vet ./...
go build -o checkfleet ./cmd/checkfleet
```

Both `go vet ./...` and `go test ./...` must be green before a change lands —
they are the same checks CI runs.

## Layout

| Path | Responsibility |
|---|---|
| `internal/engine/` | The contract: `Check`, `Finding` (status OK/WARN/BAD/ERROR), `Run` (timeout + worst-first sort), `Summarize`/`Worst`, and the typed YAML config with defaults (`LoadConfig`). |
| `internal/output/` | Renderers: `Text`, `Markdown`, `JSON`. |
| `internal/checks/<name>/` | One package per module, implementing `engine.Check`. |
| `internal/inventory/` | Minimal Ansible INI inventory parser. |
| `cmd/checkfleet/` | The CLI (stdlib `flag`, subcommands). |

## Adding a module

1. Create `internal/checks/<name>/` implementing `engine.Check`.
2. Add its typed config to `internal/engine/config.go` (with defaults in
   `LoadConfig`).
3. Wire it into `cmd/checkfleet/main.go`.
4. **Test it against a local fixture server** — an `httptest` server, or a TLS
   cert generated on the fly with a known expiry. A test that touches the
   internet or real infrastructure is a bug.

## Conventions

- Status `ERROR` means "the check could not measure" (network, handshake) — not
  "the target is unhealthy" (that's `BAD`).
- The worst-first, stable finding sort is a de-facto API — don't break it.
- The only dependency is `gopkg.in/yaml.v3`; add others only with strong
  justification.
- Todos live in
  [`BACKLOG.md`](https://github.com/Allan-Nava/checkfleet/blob/main/BACKLOG.md)
  with stable `CF-n` ids — not scattered in code comments.

## Backlog ↔ GitHub issues

`BACKLOG.md` is the single source of truth, and issues are derived from it —
never the other way around.

- `internal/backlog` parses the file into `CF-n` items (tested).
- `cmd/backlog-sync` turns each item into a GitHub issue: label `backlog`,
  grouped by milestone (the `##` sections). Checking an item (`[x]`) closes its
  issue; unchecking reopens it. Matching is by the `CF-n` title prefix, so the
  sync is **idempotent**.
- The `Backlog sync` workflow runs it on every push to `main` that touches
  `BACKLOG.md` (or the tool). Run it by hand with:

  ```bash
  go run ./cmd/backlog-sync -dry-run   # preview
  go run ./cmd/backlog-sync            # apply (needs an authenticated gh)
  ```

Don't open or close backlog issues by hand — edit `BACKLOG.md` instead.

## Releasing

Every `vX.Y.Z` tag triggers `.github/workflows/release.yml`, which runs
[goreleaser](https://goreleaser.com) (`.goreleaser.yaml`): cross-platform
archives (`linux`/`darwin`/`windows` × `amd64`/`arm64`), `checksums.txt`,
GitHub release notes from the commit log, and a Homebrew cask.

Validate the config locally without publishing:

```bash
goreleaser check                       # lint the config
goreleaser release --snapshot --clean  # full build + archives into dist/, no upload
```

**Enabling the Homebrew tap** (one-time): the cask is generated but not pushed
(`skip_upload: "true"`). To publish it:

1. create the repo `Allan-Nava/homebrew-tap`;
2. add a repo secret `HOMEBREW_TAP_GITHUB_TOKEN` (a PAT with `repo` scope);
3. set `skip_upload: "false"` in `.goreleaser.yaml`.

Then `brew install Allan-Nava/tap/checkfleet` works after the next tag.
