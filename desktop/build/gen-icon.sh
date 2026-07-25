#!/usr/bin/env bash
# Regenerate the desktop app icon from the SVG source of truth.
#
# Produces build/appicon.png (the 1024 master Wails uses) AND a *complete*
# macOS iconset (.icns) with every size at 1x and 2x.
#
# The SVG is a rounded-rect plate whose corners are TRANSPARENT. qlmanage bakes
# those corners white, which shows as an ugly white square around the icon in
# the Dock/Finder — so we render with headless Chrome, which preserves the
# alpha channel. sips/iconutil then downscale while keeping transparency.
#
# Usage:  desktop/build/gen-icon.sh
# Deps:   Google Chrome (or Chromium) + sips + iconutil.
set -euo pipefail
cd "$(dirname "$0")/.."   # -> desktop/

SVG="$PWD/frontend/dist/assets/logo.svg"
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT

# locate a Chromium-family browser (renders SVG with a transparent background)
CHROME="$(command -v google-chrome 2>/dev/null || command -v chromium 2>/dev/null || true)"
if [ -z "$CHROME" ]; then
  for p in "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
           "/Applications/Chromium.app/Contents/MacOS/Chromium"; do
    [ -x "$p" ] && CHROME="$p" && break
  done
fi
[ -n "$CHROME" ] || { echo "error: Google Chrome/Chromium not found (needed for transparent rendering)"; exit 1; }

# 1) render a crisp 1024 master with TRANSPARENT corners
cat > "$TMP/wrap.html" <<EOF
<!doctype html><html><head><meta charset="utf-8"><style>
html,body{margin:0;padding:0;background:transparent}img{display:block;width:1024px;height:1024px}
</style></head><body><img src="file://$SVG"></body></html>
EOF
"$CHROME" --headless=new --disable-gpu --hide-scrollbars --force-device-scale-factor=1 \
  --default-background-color=00000000 --window-size=1024,1024 \
  --screenshot="$TMP/master.png" "file://$TMP/wrap.html" >/dev/null 2>&1
[ -f "$TMP/master.png" ] || { echo "error: render failed"; exit 1; }
cp "$TMP/master.png" build/appicon.png
echo "• build/appicon.png regenerated (1024x1024, transparent corners)"

# 2) assemble a complete iconset (16/32/128/256/512, each 1x + 2x)
ISET="$TMP/icon.iconset"; mkdir -p "$ISET"
gen() { sips -z "$2" "$2" "$TMP/master.png" --out "$ISET/$1" >/dev/null 2>&1; }
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
