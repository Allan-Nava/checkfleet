<!--
By opening this PR you agree your contribution ships under the project's licence
(PolyForm Noncommercial 1.0.0), including in commercially-licensed copies.
See CONTRIBUTING.md — there is no CLA.
-->

## What and why

<!-- What changes, and the reasoning. The "why" is the part nobody can
reconstruct in six months — it ends up in the changelog. -->

Closes: <!-- CF-n from BACKLOG.md, or an issue number -->

## Gate

- [ ] `go vet ./...`
- [ ] `go test ./...`
- [ ] `golangci-lint run` (**v2** — a v1 binary silently ignores `.golangci.yml`)
- [ ] `cd desktop && go test ./...` *(only if `desktop/` was touched)*

## Checks

- [ ] Tests run offline — fixture servers created in-test, never real infrastructure or the internet
- [ ] No secrets in code, tests, example configs, docs or output
- [ ] User-facing text is in English (findings, usage, flag help, errors)
- [ ] Exit code semantics unchanged: `0` even with WARN/BAD findings, non-zero only for systemic failures
- [ ] Nothing on the [compatibility contract](../docs/compatibility.md) broke — or if it had to, the doc and the deprecation path are updated in this PR

<!-- Don't bump the version or edit CHANGELOG.md: releases are tagged by the maintainer. -->
