# Backlog — checkfleet

Sorgente unica dei todo. Id stabili `CF-n`; spuntare, non cancellare.

> **Sync automatico issue**: questo file è la fonte di verità. Ogni `CF-n` diventa una issue GitHub (label `backlog`, milestone per sezione) via `cmd/backlog-sync` + workflow `.github/workflows/backlog-sync.yml`. Spuntare un item (`[x]`) chiude la issue al prossimo push; toglierlo la riapre. Idempotente. Non aprire/chiudere le issue a mano: edita qui.

Roadmap a milestone: prima **cosa monitorare** (M1→M3), poi **come consegnarlo** (M4) e **come usarlo** (M5). Le versioni sono indicative: ogni modulo/output è comunque una release taggata a sé.

## M1 — Rete & delivery (~v0.2) — il cuore hiway media

- [x] **CF-1 — Modulo `nats`**: preflight/health cluster NATS via monitor endpoint (`/varz`, `/jsz?meta=1`): meta-leader presente e in posizione, ghost peer, lag peer oltre soglia, versioni miste nel cluster. Codifica il runbook devops_hiway. _(v0.2.0; lag misurato sui peer del raft meta-group)_
- [x] **CF-2 — Modulo `haproxy`**: backend/server DOWN via CSV stats HTTP; MAINT/DRAIN/NOLB → WARN, backend senza server → BAD, saturazione sessioni opzionale, basic-auth con password da env. _(v0.3.0; drift config running vs file rimandato)_
- [x] **CF-3 — Modulo `stream`**: HLS/DASH — manifest raggiungibile/valido, ladder completa (varianti/representation), freschezza live-edge via `EXT-X-PROGRAM-DATE-TIME`/`publishTime`, live vs VOD. _(v0.4.0; solo manifest, mai i segmenti)_

## M2 — Data layer (~v0.3)

- [x] **CF-4 — Modulo `patroni`**: leader per cluster via REST API Patroni (`/cluster`), repliche in lag, stato replica, divergenza timeline. _(v0.5.0; split-brain → WARN, no leader → BAD)_
- [x] **CF-11 — Modulo `postgres`**: reachability, età transazione (wraparound), connessioni vs `max_connections`, replication slot inattivi che trattengono WAL, replica lag (su primary via `pg_stat_replication`). Solo lettura, driver `pgx`. _(v0.7.0; logica testata con DB finto, mai infra reale)_
- [x] **CF-12 — Modulo `consul`**: quorum raft e leader presente, servizi in stato `critical`/`warning`, sanity KV su chiavi note, ACL token da env. _(v0.6.0; membri failed/left rimandati)_

## M3 — Piattaforma & host (~v0.4) — check leggeri, valore di dominio

- [x] **CF-13 — Modulo `dns`**: record risolvono, drift vs valore atteso, SOA/serial coerente tra i resolver, TTL sotto soglia. Client DNS minimale in-tree (zero dip), A/AAAA/CNAME/NS/TXT/SOA. _(v0.8.0)_
- [x] **CF-14 — Modulo `endpoint`/`disk`**: ~~spazio su path critici e stato host agentless via SSH~~. **Deciso: non fare.** Disco/host si delegano a node_exporter + alerting: SSH agentless tradirebbe il "no agent" e duplicherebbe Prometheus. Se dovesse servire, riaprire con motivazione. _(deciso 2026-07-24)_

## M4 — Output & integrazione (~v0.5)

- [x] **CF-5 — Output Slack** (Block Kit): `--output slack --webhook-env SLACK_WEBHOOK` con summary + problemi (worst-first, cap 20). Webhook da env, mai in CLI/config. _(v0.9.0)_
- [x] **CF-6 — Modalità exporter Prometheus**: `checkfleet serve --listen :9876 --interval 60s` espone i finding come metriche (gauge severity per check/target + rollup) rieseguendo i check a intervallo. _(v0.10.0; registry moduli condiviso check/serve)_
- [x] **CF-7 — Findings → issue GitHub**: `checkfleet report-issues` apre una issue per ogni finding BAD/ERROR (dedup per check+target) e la chiude al rientro. Idempotente, `--dry-run`, label `checkfleet-finding`. GitLab dietro interfaccia (rimandato). _(v0.12.0)_
- [x] **CF-8 — Config multi-stack**: `--stack prod-cologno` sovrappone `checkfleet.<stack>.yml` alla base (merge per modulo, timeout se impostato). Vale per `check` e `serve`. _(v0.11.0)_

## M5 — App desktop Wails (~v0.6) — "così è più semplice da usare"

Stack scelto: **Wails** (core Go che riusa direttamente `internal/engine`, frontend web leggero, binario singolo — coerente con la filosofia zero-dep). Il core CLI resta la fonte di verità: la GUI è un frontend, non una fork della logica.

- [ ] **CF-15 — Scaffold Wails**: progetto Wails in `desktop/` che importa `internal/engine`/`internal/output` senza duplicare logica; build separata dal binario CLI, stessa versione via ldflags. La CI CLI non deve dipendere dalla toolchain web.
- [ ] **CF-16 — Vista fleet**: carica `checkfleet.yml`, esegue i check, mostra summary + tabella finding con stesso sort worst-first; colori per status (OK/WARN/BAD/ERROR).
- [ ] **CF-17 — Run & refresh**: bottone "run", auto-refresh a intervallo, selettore stack (dipende da CF-8), export markdown/JSON riusando `internal/output`.
- [ ] **CF-18 — Packaging desktop**: bundle macOS (`.app`)/Linux, icona, firma dove serve; separato dalla release CLI di CF-9.

## Rilascio

- [x] **CF-9 — goreleaser** (linux/darwin/windows, amd64/arm64) + archivi + checksums + Homebrew cask. Tap pronto ma disattivo (`skip_upload`) finché non si crea `Allan-Nava/homebrew-tap` + secret. Validato con `goreleaser check` + `--snapshot`. _(v0.13.0)_
- [ ] **CF-10 — Docs sito** (README ricco con GIF, esempi per modulo, ricette CI TeamCity/GitHub Actions).
