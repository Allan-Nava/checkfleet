---
title: Development
nav_order: 9
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
| `internal/engine/` | The contract: `Check`, `Finding` (status OK/WARN/BAD/ERROR), `Run` (checks run **concurrently**, each with its own timeout; results flattened in check order then worst-first sorted → deterministic), `Summarize`/`Worst`, and the typed YAML config with defaults (`LoadConfig`). |
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

## Integration suite (opt-in)

Unit tests must stay offline (in-test servers only). To exercise the modules
against **real** services there's a separate, opt-in suite gated behind the
`integration` build tag, so `go test ./...` never runs it.

```bash
docker compose -f docker-compose.integration.yml up -d --build --wait
go test -tags integration -v ./test/integration/...
checkfleet check all --config checkfleet.integration.yml   # end-to-end smoke
docker compose -f docker-compose.integration.yml down -v
```

- `docker-compose.integration.yml` brings up redis, nats, consul, haproxy,
  postgres, patroni (+etcd), and keycloak, each published on `127.0.0.1` with a
  healthcheck so `--wait` makes readiness deterministic. Support files live in
  `deploy/integration/` (HAProxy config, the in-compose Patroni image).
- `checkfleet.integration.yml` points every module at those local ports.
- The suite's contract is deliberately loose: it asserts **reachability** — at
  least one non-`ERROR` finding per module — not exact status (that stays
  covered by the unit tests). NATS runs standalone, so its meta-cluster finding
  is `BAD` by design (a single node is not an HA cluster); it's exit-neutral.
- CI runs all of this in `.github/workflows/integration.yml`, a job kept
  separate from the `test` job in `ci.yml`.

## Fuzzing the parsers

The parsers that read untrusted external input are fuzzed (CF-36): `parseM3U8`
(HLS manifests), `parseMessage` (the hand-rolled DNS wire decoder), `parseCSV`
(HAProxy stats), and the `/jsz` decode + meta-cluster analysis (NATS). Each has
a white-box `Fuzz*` target in its package.

```bash
go test ./internal/checks/dns -run '^$' -fuzz '^FuzzParseMessage$' -fuzztime=30s
```

The seed corpora run as ordinary tests under `go test ./...`, so a known
crasher can never regress. The [`Fuzz`](https://github.com/Allan-Nava/checkfleet/actions/workflows/fuzz.yml)
workflow fuzzes every target on a schedule, on demand, and on PRs that touch a
parser; any crasher lands in `internal/checks/<mod>/testdata/fuzz/` and is
uploaded as an artifact.

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

**Homebrew tap**: enabled. On every `v*` tag goreleaser pushes the cask to
[`Allan-Nava/homebrew-tap`](https://github.com/Allan-Nava/homebrew-tap) (the
repo and the `HOMEBREW_TAP_GITHUB_TOKEN` secret are set up, `skip_upload:
"false"`), so:

```bash
brew install Allan-Nava/tap/checkfleet
```

The cask ships the release archive's prebuilt binary (darwin amd64/arm64) and
strips the `com.apple.quarantine` attribute on install (the binary is
unsigned). Only tags *after* the tap was enabled carry the cask — older
releases won't install via brew.
