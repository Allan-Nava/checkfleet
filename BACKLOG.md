# Backlog — checkfleet

Sorgente unica dei todo. Id stabili `CF-n`; spuntare, non cancellare.

## v0.2 — Moduli core

- [ ] **CF-1 — Modulo `nats`**: preflight/health cluster NATS via monitor endpoint (`/varz`, `/jsz?meta=1`): meta-leader presente e in posizione, ghost peer, lag consumer oltre soglia, versioni miste nel cluster. Codifica il runbook devops_hiway.
- [ ] **CF-2 — Modulo `haproxy`**: backend/server DOWN via stats socket o API; opzionale drift config running vs file di riferimento.
- [ ] **CF-3 — Modulo `stream`**: HLS/DASH — manifest raggiungibile, freschezza segmenti live, ladder completa, drift del live edge.
- [ ] **CF-4 — Modulo `patroni`**: leader per cluster (API Patroni o Consul), repliche in lag.

## v0.3 — Output & integrazione

- [ ] **CF-5 — Output Slack** (Block Kit): `--output slack --webhook-env SLACK_WEBHOOK` con summary + problemi.
- [ ] **CF-6 — Modalità exporter Prometheus**: `checkfleet serve --listen :9876` che espone i finding come metriche (gauge per status) rieseguendo i check a intervallo.
- [ ] **CF-7 — Findings → issue GitHub/GitLab**: apre/aggiorna una issue per i finding BAD persistenti (dedup per check+target).
- [ ] **CF-8 — Config multi-stack**: più file/profili (`--stack prod-cologno`) con merge dei default.

## Rilascio

- [ ] **CF-9 — goreleaser o build matrix completa** (linux/darwin, amd64/arm64) + Homebrew tap.
- [ ] **CF-10 — Docs sito** (README ricco con GIF, esempi per modulo, ricette CI TeamCity/GitHub Actions).
