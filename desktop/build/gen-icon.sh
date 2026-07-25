#!/usr/bin/env bash
# Regenerate the desktop app icon from the SVG source of truth.
#
# Produces build/appicon.png (the 1024 master Wails uses) AND a *complete*
# macOS iconset (.icns) with every size at 1x and 2x — Wails' own generation
# only emits the @2x reps, so run this after `wails build` when you want the
# full set installed in the bundle.
#
# Usage:  desktop/build/gen-icon.sh
# Deps:   qlmanage + sips + iconutil (all stock macOS).
set -euo pipefail
cd "$(dirname "$0")/.."   # -> desktop/

SVG="frontend/dist/assets/logo.svg"
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT

# 1) render a crisp 1024 master straight from the vector
qlmanage -t -s 1024 -o "$TMP" "$SVG" >/dev/null 2>&1
MASTER="$TMP/$(basename "$SVG").png"
[ -f "$MASTER" ] || { echo "error: SVG render failed"; exit 1; }
cp "$MASTER" build/appicon.png
echo "• build/appicon.png regenerated (1024x1024)"

# 2) assemble a complete iconset (16/32/128/256/512, each 1x + 2x)
ISET="$TMP/icon.iconset"; mkdir -p "$ISET"
gen() { sips -z "$2" "$2" "$MASTER" --out "$ISET/$1" >/dev/null 2>&1; }
gen icon_16x16.png 16
gen icon_16x16@2x.png 32
gen icon_32x32.png 32
gen icon_32x32@2x.png 64
gen icon_128x128.png 128
gen icon_128x128@2x.png 256
gen icon_256x256.png 256
gen icon_256x256@2x.png 512
gen icon_512x512.png 512
gen icon_512x512@2x.png 1024
iconutil -c icns "$ISET" -o "$TMP/iconfile.icns"
echo "• complete iconfile.icns built (10 reps: 16→1024, 1x+2x)"

# 3) install into the built bundle if present
APP="build/bin/checkfleet.app/Contents/Resources/iconfile.icns"
if [ -f "$APP" ]; then
  cp "$TMP/iconfile.icns" "$APP"
  echo "• installed into build/bin/checkfleet.app"
else
  echo "• (no built .app yet — run 'wails build' first to install into the bundle)"
fi
