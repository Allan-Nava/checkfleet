---
title: Desktop app
nav_order: 7
---

A small desktop GUI over the same engine as the CLI. It's a [Wails](https://wails.io)
app — a single native binary (macOS `.app`, Linux, Windows) with the web
frontend embedded — that **reuses `internal/engine`**: the checks, the
worst-first sort and the findings are identical to `checkfleet check all`. The
CLI stays the source of truth; the GUI is just another frontend.

![checkfleet desktop — fleet view](assets/desktop-dark.png)

## The fleet view

Everything is one screen, scanned top-to-bottom.

**Toolbar**

- **Config** — the `checkfleet.yml` to run. Type a path or **Browse…** for a
  native file picker.
- **Stack** — pick a `checkfleet.<stack>.yml` profile discovered next to the
  config (same overlay as the CLI's `--stack`); `(base)` runs the base file.
- **Auto** + interval — re-run the checks on a timer (10s / 30s / 60s / 5m).
- **Notify** — pop a native OS notification after a run whose worst status is
  BAD/ERROR.
- **Run** — execute every configured module now.

The app remembers your config path, stack, interval, Auto and Notify between
launches.

**Summary**

- The **worst status** pill (OK / WARN / BAD / ERROR) — the one thing to read
  first.
- Count tiles for **OK / WARN / BAD / ERROR**.
- Total findings, run **duration**, run time, and a chip per configured module.

**Findings table**

- One row per finding, **worst-first** (same order as the CLI), with a colored
  status badge and the `Status / Check / Target / Message` columns.
- **Filter** box — live substring match over check, target and message.
- **Severity** dropdown — show all, or only `≥ WARN`, `≥ BAD`, `ERROR`.

**Export**

- **Export** — pick a format (Markdown, JSON, HTML, JUnit, Prometheus, OTLP) and
  save the current run via a native save dialog. Same renderers as the CLI's
  `--output`.

## Details, validate & explain

Click a finding row to open a **detail drawer** with the full message and a
**Copy** button. Click a **module chip** in the summary to see what that check
verifies (**Explain**), and the **Validate** button checks the config without
running anything (the same problems as `checkfleet validate`).

![checkfleet desktop — finding detail drawer](assets/desktop-detail.png)

## Changes since the last run

After a second run, **Changes (N)** opens a drawer with only what moved — new,
resolved, worsened or improved findings — so you see the delta at a glance
during an incident (in-session, no history file needed).

![checkfleet desktop — changes drawer](assets/desktop-changes.png)

## Edit the config

The gear button (⚙, top-right) opens a full-panel YAML editor on the selected
`checkfleet.yml`:

- **Reload** — re-read the file from disk, discarding unsaved edits.
- **Validate** — check the *unsaved* text (YAML parse + domain rules) and list
  any problems inline, without saving. This runs the same validation as the CLI.
- **Save** — write the text back to the file.

Once saved, run the fleet from the GUI, or point cron / `checkfleet serve
--interval` at the same file — the config is the single source, and the app is
just one way to edit and run it.

![checkfleet desktop — config editor](assets/desktop-config.png)

## Light theme

The theme toggle (top-right) switches light/dark and remembers your choice.

![checkfleet desktop — light theme](assets/desktop-light.png)

## Open straight into a fleet

Two environment variables let the app open on a config and run immediately —
handy for an "open with" launcher or a kiosk view:

```bash
CHECKFLEET_CONFIG=/etc/checkfleet.yml CHECKFLEET_AUTORUN=1 checkfleet-desktop
```

Without them the app opens on `./checkfleet.yml` (if present) and waits for you
to press **Run**.

## Get it

Every [release](https://github.com/Allan-Nava/checkfleet/releases) attaches the
desktop builds next to the CLI archives:

- `checkfleet-desktop_<version>_darwin_universal.zip` — macOS `.app` (Intel +
  Apple Silicon)
- `checkfleet-desktop_<version>_linux_amd64.tar.gz`
- `checkfleet-desktop_<version>_windows_amd64.zip`

> The desktop binaries are unsigned for now — on macOS, right-click → **Open**
> the first time (or clear the quarantine attribute).

### Build from source

Requires the [Wails v2 toolchain](https://wails.io/docs/gettingstarted/installation)
and its platform prerequisites (macOS: Xcode command-line tools; Linux:
`libgtk-3` + `libwebkit2gtk-4.1`). Node is not needed — the frontend is static.

```bash
cd desktop
go mod tidy
wails dev                                   # hot-reload dev app
wails build -platform darwin/universal      # or linux/amd64, windows/amd64
```

The app lives in [`desktop/`](https://github.com/Allan-Nava/checkfleet/tree/main/desktop)
as a separate Go module, so the Wails toolchain never enters the CLI's build.
