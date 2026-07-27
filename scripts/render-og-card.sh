#!/usr/bin/env bash
# Render the OpenGraph/Twitter social card from scripts/og-card.html.
#
# The card is what GitHub, Slack, X, LinkedIn and Discord show when someone
# shares a checkfleet link, so it is checked in as a PNG rather than generated
# at deploy time. Re-run this after editing og-card.html (e.g. when the module
# count changes) and commit the result.
#
# Usage: scripts/render-og-card.sh
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
src="$root/scripts/og-card.html"
out="$root/docs/assets/og-card.png"

chrome=""
for candidate in \
  "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
  "/Applications/Chromium.app/Contents/MacOS/Chromium" \
  "$(command -v google-chrome || true)" \
  "$(command -v chromium || true)" \
  "$(command -v chromium-browser || true)"; do
  if [ -n "$candidate" ] && [ -x "$candidate" ]; then chrome="$candidate"; break; fi
done

if [ -z "$chrome" ]; then
  echo "render-og-card: no Chrome/Chromium found; install one or render og-card.html by hand at 1200x630" >&2
  exit 1
fi

"$chrome" --headless=new --disable-gpu --hide-scrollbars \
  --force-device-scale-factor=1 --window-size=1200,630 \
  --screenshot="$out" "file://$src" >/dev/null 2>&1

echo "wrote $out ($(wc -c <"$out" | tr -d ' ') bytes, 1200x630)"
