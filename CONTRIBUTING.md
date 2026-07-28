# Contributing to checkfleet

Thanks for looking. A few things worth knowing before you spend time on a change.

## First, the honest part

checkfleet is maintained by **one person** and licensed under [PolyForm Noncommercial 1.0.0](LICENSE) — source-available, free for noncommercial use, commercial use requires a [separate license](COMMERCIAL.md). Two consequences:

- **Response times are best-effort.** A PR may sit for a while. If that is a problem for you, say so in the description and I will tell you honestly whether I can get to it.
- **By opening a PR you agree your contribution ships under the project's license**, including in commercially-licensed copies. There is no CLA to sign; this paragraph is the whole arrangement. If that does not work for you, please don't send code — an issue describing the problem is genuinely just as useful.

**Open an issue before a large change.** A new module is welcome; a new module built against the wrong contract, or one I have already decided not to do (see [`BACKLOG.md`](BACKLOG.md), which records rejected ideas *with the reasoning* — `CF-14` for instance), is wasted work for both of us.

## The gate

Every change must pass what CI passes. All three, not two:

```bash
go vet ./...
go test ./...
golangci-lint run
```

The linter is a **hard gate** in CI, and leaving it out of a local checklist has already let 20 consecutive red runs through. It must be **golangci-lint v2** — `.golangci.yml` is a v2-schema config and a v1 binary silently fails to read it. Same version CI pins:

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2
```

Go 1.25 or later (see `go.mod`).

The desktop app is a **separate Go module** in `desktop/`, excluded from `./...`. If you touch it, run its tests too:

```bash
cd desktop && go test ./...
```

## Rules that are not negotiable

These come from how the tool is used, and a PR that breaks one will be asked to change:

1. **Tests never touch the network or real infrastructure.** Every module is tested against a server created *in the test* — an `httptest` server, a TLS cert generated on the fly with a known expiry, a fake protocol server. A test that reaches the internet is a bug, not a stronger test: it fails in someone's CI at 3am for reasons unrelated to your change.

2. **Exit code semantics do not change.** A run exits `0` even with WARN and BAD findings — a check that ran *is* a success — and non-zero is reserved for systemic failures. Gating is opt-in via `--exit-on`. This is written down in [`docs/compatibility.md`](docs/compatibility.md) and load-bearing for every pipeline using checkfleet.

3. **`ERROR` is not `BAD`.** `ERROR` means checkfleet could not measure (network, handshake, timeout). `BAD` means the target is unhealthy. Conflating them pages someone about a firewall rule as if the database were down.

4. **No secrets anywhere.** Not in example configs, tests, docs, or output. Credentials come from the environment (`${VAR}`, `${file:/path}`); checks never log them. Where a target is a DSN, only the host and port get rendered.

5. **Zero dependencies is the default.** HTTP/JSON and hand-written simple protocols need no driver. A new dependency needs a strong reason — the existing exceptions (`pgx`, `go-ldap`, `franz-go`, `mongo-driver`, `go-sql-driver/mysql`) are all protocols that are genuinely unreasonable to write by hand, and each is argued for in [`CLAUDE.md`](CLAUDE.md).

6. **Everything user-facing is in English** — code, comments, tests, finding messages, usage, flag help, errors. `scripts/check-english.sh` enforces it in CI. (`CHANGELOG.md` is in Italian; that is a deliberate project exception.)

7. **Anything on the [compatibility contract](docs/compatibility.md) stays stable.** If your change touches a documented JSON key, metric name, config key or exit code, the contract tests will fail — and they are meant to. Update the doc in the same commit, and follow the deprecation policy rather than breaking it.

8. **Todos live in [`BACKLOG.md`](BACKLOG.md)** with stable `CF-n` ids, not scattered as `TODO` comments in the code.

## Adding a module

The shape every module follows:

1. A package in `internal/checks/<name>/` implementing `engine.Check`.
2. Typed config in `internal/engine/config.go`, with defaults.
3. Wiring in `internal/registry` so both the CLI and the desktop see it.
4. Docs in `internal/moduledoc` (the single source behind `checkfleet explain`, the SARIF rules and the desktop) and an entry in `docs/_data/modules.yml`.
5. Tests against a fixture server created in-test.

Full walkthrough with the `engine.Check` contract: [`docs/development.md`](https://allan-nava.github.io/checkfleet/development/).

The bar for a module is not "it connects". It is: **does it know something a port check doesn't?** A module should encode the domain knowledge an operator has — that a Patroni cluster with two leaders is worse than one with none, that an HLS manifest can be reachable and stale, that a replication slot nobody consumes will eventually fill the disk. If a `tcp` check would tell you the same thing, the module isn't earning its place.

## Pull requests

- One logical change per PR. Every release is tagged and gets a `CHANGELOG.md` entry, so a PR doing three things becomes three awkward entries.
- Say **why**, not just what. The changelog entries in this repo are long on purpose: six months from now the reasoning is the part nobody can reconstruct.
- Tests in the same PR as the code.
- Don't bump the version or edit `CHANGELOG.md` — releases are tagged by the maintainer.

## Reporting a bug

Include `checkfleet version`, the config that reproduces it (**secrets redacted**), the command, and what you expected instead. `checkfleet doctor --config …` output is often the fastest way to a diagnosis.

For a **security** issue, don't open an issue — see [`SECURITY.md`](SECURITY.md).
