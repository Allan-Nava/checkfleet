# checkfleet desktop (Wails)

A thin desktop GUI over the checkfleet engine. It **reuses** `internal/engine`,
`internal/registry` and `internal/output` directly — the CLI stays the source of
truth, the GUI is just another frontend (CF-15).

This is a **separate Go module** on purpose: the Wails/web toolchain and its
large dependency tree stay out of the CLI module, so `go build ./...` and CI at
the repo root never pull them in.

## What it does

- **Fleet view** (CF-16): load a `checkfleet.yml`, run every configured module,
  show a summary (worst status + OK/WARN/BAD/ERROR tiles + module chips) and the
  findings table, sorted worst-first, with per-status colors.
- **Run & refresh** (CF-17): a Run button, optional auto-refresh on an interval,
  a stack selector (discovers `checkfleet.<stack>.yml` beside the config), live
  filtering + min-severity, and export to Markdown/JSON via `internal/output`.

## Develop

Requires the [Wails v2 toolchain](https://wails.io/docs/gettingstarted/installation)
(`go install github.com/wailsapp/wails/v2/cmd/wails@latest`) plus its platform
prerequisites (on macOS: Xcode command-line tools; on Linux: `libgtk-3` +
`libwebkit2gtk-4.0`). Node is **not** required — the frontend is static.

```bash
cd desktop
go mod tidy          # resolves the Wails dependency tree (first run only)
wails dev            # hot-reload dev app
```

The frontend is plain HTML/CSS/JS under `frontend/dist/` — no bundler, no build
step. `wails.json` has empty `frontend:install`/`frontend:build`. Edit those
files directly.

## Build & package (CF-18)

```bash
# macOS universal .app
wails build -platform darwin/universal \
  -ldflags "-X main.version=$(git describe --tags --always)"

# Linux
wails build -platform linux/amd64 \
  -ldflags "-X main.version=$(git describe --tags --always)"
```

Output lands in `build/bin/`. The app icon is `build/appicon.png` (generated
from `docs/assets/logo.svg`). Packaging is intentionally **separate** from the
CLI's goreleaser flow (CF-9): the desktop build needs the web toolchain, the CLI
release must not. A manual GitHub Actions pipeline lives at
`.github/workflows/desktop.yml` (dispatch-only, so it never runs on CLI tags).

## Preview without the toolchain

`frontend/dist/index.html` opened in a browser renders with realistic **mock**
data (the Go bindings are absent), so the UI can be reviewed without building.
