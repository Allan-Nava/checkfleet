---
title: Installation
---

[← back to index](index.md)

# Installation

## With `go install`

```bash
go install github.com/Allan-Nava/checkfleet/cmd/checkfleet@latest
```

The binary lands in `$(go env GOPATH)/bin`. Make sure that directory is on your
`PATH`.

## From a release binary

Each `vX.Y.Z` tag publishes static binaries for `linux` and `darwin`, on both
`amd64` and `arm64`, plus a `SHA256SUMS` file. Download the one for your
platform from the
[releases page](https://github.com/Allan-Nava/checkfleet/releases), verify it,
and drop it on your `PATH`:

```bash
shasum -a 256 -c SHA256SUMS      # verify against the published checksums
chmod +x checkfleet_*            # make it executable
sudo mv checkfleet_* /usr/local/bin/checkfleet
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
