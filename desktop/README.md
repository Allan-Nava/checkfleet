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
step. `wails.json` has empty `frontend:install`/`frontend:build`, and an **empty
`frontend:dev:serverUrl`** so `wails dev` serves `frontend/dist/` directly and
hot-reloads on save (there is no dev server to auto-discover). Edit the files
directly.

`wails dev` also serves the app in a browser at **http://localhost:34115** with
full DevTools *and* the real Go bindings (`window.go.main.App`), so you get real
data (not the preview mock) and a console to debug from. Set `CHECKFLEET_CONFIG`
(and `CHECKFLEET_AUTORUN=1`) before `wails dev` to open straight into a fleet.

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
release must not.

`.github/workflows/desktop.yml` builds the app for macOS/Linux/Windows on every
`v*` tag and **attaches the executables to that GitHub Release** (the one
goreleaser creates) — it waits for the release to exist, then uploads:

- `checkfleet-desktop_<version>_darwin_universal.zip` (the `.app`)
- `checkfleet-desktop_<version>_linux_amd64.tar.gz`
- `checkfleet-desktop_<version>_windows_amd64.zip`

Because it runs as its own workflow, a desktop build failure never blocks the
CLI release. It can also be run by hand (`workflow_dispatch`), which uploads the
same files as workflow artifacts instead.

## Preview without the toolchain

`frontend/dist/index.html` opened in a browser renders with realistic **mock**
data (the Go bindings are absent), so the UI can be reviewed without building.
