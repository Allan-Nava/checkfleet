#!/usr/bin/env bash
# Guardrail (CF-60): the software's Go source must not carry Italian in
# user-facing strings/comments. Output was migrated to English (M15); this keeps
# it from regressing. Scans cmd/ and internal/ .go files for accented Italian
# vowels and a denylist of distinctive Italian words.
#
# NOT scanned: CHANGELOG.md (intentionally Italian) and BACKLOG.md (planning).
set -euo pipefail
cd "$(dirname "$0")/.."

paths=(cmd internal)
fail=0

# 1) Accented Italian vowels never appear in the (now English) code.
if LC_ALL=en_US.UTF-8 grep -rnE 'à|è|é|ì|ò|ù' --include='*.go' "${paths[@]}"; then
  echo "::error::accented Italian vowel found in Go source (see above)"
  fail=1
fi

# 2) Distinctive Italian words that shouldn't occur in English code.
words='nessun|nessuna|atteso|attesa|avuto|soglia|fallit|raggiungibil|sconosciut|richiesta|connessione|configurazione|inatteso|mancante|mancanti|giorni|messaggi|ritardo|trattenut|misurabile|varianti|segmenti|risposta|vuota|presente|divergent[ei]|invalido|invalida'
if grep -rniE "\\b(${words})\\b" --include='*.go' "${paths[@]}"; then
  echo "::error::Italian word found in Go source (see above) — keep output English (M15)"
  fail=1
fi

if [ "$fail" -eq 0 ]; then
  echo "check-english: OK — no Italian found in cmd/ or internal/"
fi
exit "$fail"
