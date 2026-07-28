---
title: Installation
nav_order: 2
description: >-
  Install checkfleet with go install, the Homebrew tap, the Docker image, or a
  release archive for Linux, macOS and Windows — then verify the binary.
---

## With `go install`

```bash
go install github.com/Allan-Nava/checkfleet/cmd/checkfleet@latest
```

The binary lands in `$(go env GOPATH)/bin`. Make sure that directory is on your
`PATH`.

## With Homebrew

```bash
brew install Allan-Nava/tap/checkfleet
# equivalent, if you prefer to tap first:
brew tap Allan-Nava/tap && brew install checkfleet
```

The cask ships the prebuilt release binary (macOS `amd64`/`arm64`) and strips
the `com.apple.quarantine` attribute on install, so it runs without a Gatekeeper
prompt. Every `v*` tag refreshes the cask automatically, and a
[`Brew test`](https://github.com/Allan-Nava/checkfleet/actions/workflows/brew-test.yml)
workflow verifies the install on both Apple Silicon and Intel after each
release.

> Homebrew 6+ may ask you to *trust* the tap the first time
> (`brew trust --cask Allan-Nava/tap/checkfleet`). That's expected for any
> third-party tap.

## From a release archive

Each `vX.Y.Z` tag publishes archives (built by goreleaser) for `linux`,
`darwin` and `windows` on both `amd64` and `arm64`, plus a `checksums.txt`.
Download the one for your platform from the
[releases page](https://github.com/Allan-Nava/checkfleet/releases), verify it,
extract, and drop the binary on your `PATH`:

```bash
sha256sum -c checksums.txt --ignore-missing        # verify
tar xzf checkfleet_*_linux_amd64.tar.gz            # extract (zip on Windows)
sudo mv checkfleet /usr/local/bin/
```

The binaries are built with `CGO_ENABLED=0` and `-trimpath`, so they are fully
static — no runtime dependencies.

## From source

```bash
git clone https://github.com/Allan-Nava/checkfleet
cd checkfleet
go build -o checkfleet ./cmd/checkfleet
```

## Checking the version

```bash
checkfleet version
```

The version string is injected at build time on tagged releases; a local
`go build` reports `dev`.

## Docker

Multi-arch images (linux/amd64+arm64) are published to GHCR on every release.
The default entrypoint is the Prometheus exporter:

```bash
# exporter (metrics on :9876) — mount your config
docker run -p 9876:9876 -v "$PWD/checkfleet.yml:/checkfleet.yml" \
  ghcr.io/allan-nava/checkfleet:latest

# one-shot check
docker run -v "$PWD/checkfleet.yml:/checkfleet.yml" \
  ghcr.io/allan-nava/checkfleet:latest check all --config /checkfleet.yml
```

## Verify a release (cosign + SBOM)

Release archives ship an SBOM (`*.sbom.json`, syft) and the `checksums.txt` is
signed with **keyless cosign**. Verify it (identity = the release workflow):

```bash
cosign verify-blob \
  --certificate checksums.txt.pem \
  --signature checksums.txt.sig \
  --certificate-identity-regexp 'https://github.com/Allan-Nava/checkfleet/.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt
```

Recent cosign versions print a deprecation notice for `--certificate` and
`--signature` (the newer form is a `--bundle`). The command above still verifies
correctly — the notice is about the flag shape, not the signature — and it is
kept as-is because it is what works against the artifacts goreleaser publishes
today. Verified against v0.142.0: `Verified OK`, identity
`release.yml@refs/tags/v0.142.0`.

Docker images are signed too:

```bash
cosign verify ghcr.io/allan-nava/checkfleet:latest \
  --certificate-identity-regexp 'https://github.com/Allan-Nava/checkfleet/.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```
