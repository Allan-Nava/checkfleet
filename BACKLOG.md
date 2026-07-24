# Backlog — checkfleet

Sorgente unica dei todo. Id stabili `CF-n`; spuntare, non cancellare.

> **Sync automatico issue**: questo file è la fonte di verità. Ogni `CF-n` diventa una issue GitHub (label `backlog`, milestone per sezione) via `cmd/backlog-sync` + workflow `.github/workflows/backlog-sync.yml`. Spuntare un item (`[x]`) chiude la issue al prossimo push; toglierlo la riapre. Idempotente. Non aprire/chiudere le issue a mano: edita qui.

Roadmap a milestone. **Fase 1 (completa)**: cosa monitorare (M1→M3), come consegnarlo (M4), rilascio. **Fase 2**: usarlo meglio (M5 app desktop), più domini (M6), più alerting (M7), engine più solido (M8), qualità (M9). Le versioni sono indicative: ogni modulo/output è comunque una release taggata a sé. **Prossimo:** M5 (Wails), poi M6.

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
- [x] **CF-10 — Docs sito**: sito con tema custom (hero, ricerca, TOC, pagine per-modulo con esempi config), ricette CI GitHub Actions + TeamCity + cron, README ricco. GIF demo rimandata (asset binario, non generabile qui; c'è la demo testuale). _(v0.14.0)_

## M6 — Più moduli di dominio (fase 2)

- [x] **CF-19 — Modulo `redis`/`valkey`**: reachability (`PING`)+`INFO` con **client RESP minimale in-tree** (zero dip), uso memoria vs `maxmemory`, replica (link up/down, offset lag), persistenza RDB/AOF, loading. TLS + ACL (password da env). _(v0.20.0)_
- [ ] **CF-20 — Modulo `keycloak`**: health/ready endpoint, token endpoint del realm risponde, certificati/allineamento issuer, versione. Nessuna credenziale in config (client-credentials da env se serve).
- [ ] **CF-21 — Modulo `mediamtx`**: API di mediamtx — path attivi, reader/publisher per path, path attesi presenti, ingest fermi. Codifica l'uso hiway (KV_mediamtx nel runbook NATS).
- [ ] **CF-22 — Modulo `ingest` (RTMP/SRT)**: l'endpoint di ingest accetta connessioni (handshake TCP/RTMP, o SRT), latenza. Segnale "lo streamer riesce a pubblicare?".
- [ ] **CF-23 — Modulo `s3`/object storage**: bucket raggiungibile, oggetto sentinella presente e fresco (last-modified sotto soglia), spazio/quota se esposta. Credenziali da env.
- [ ] **CF-24 — Modulo `smtp`**: il relay accetta connessioni, STARTTLS ok, cert del relay non scaduto, banner atteso. Nessun invio reale.
- [ ] **CF-25 — Modulo `elasticsearch`/`opensearch`**: `_cluster/health` (green/yellow/red), shard unassigned, nodi attesi presenti, disk watermark.

## M7 — Alerting & output (fase 2)

- [ ] **CF-26 — Issue GitLab**: implementa `issuesync.Client` per GitLab (via `glab`/API), stessa logica di CF-7. Selezione forge in config/flag.
- [ ] **CF-27 — Webhook Discord/Teams**: output verso Discord/Teams (come Slack), payload adatti, URL da env.
- [ ] **CF-28 — Alert PagerDuty/Opsgenie**: crea/risolve alert per finding BAD/ERROR (dedup per check+target), chiave di routing da env.
- [ ] **CF-29 — Report HTML**: `--output html` — report statico autoconsistente (summary, "Da guardare", tabella), tema coerente col sito.
- [ ] **CF-30 — Export OTLP**: esporta i finding come metriche/eventi OpenTelemetry (OTLP), per chi non usa lo scrape Prometheus.

## M8 — Engine & UX (fase 2)

- [x] **CF-31 — Check concorrenti**: il runner esegue i moduli in parallelo (goroutine per check, timeout per-check), raccolta per-indice + sort stabile → output deterministico e wall-clock ≈ check più lento. _(v0.15.0)_
- [x] **CF-32 — Storico & flap/trend**: persistenza dei run in **JSONL** (zero dip, scelto vs SQLite); `--history <file>` registra il run e aggiunge finding `flap` (WARN) per i target che cambiano stato ≥ soglia nella finestra recente. Package `internal/history`. _(v0.19.0; SQLite scartato per la filosofia zero-dep)_
- [x] **CF-33 — `checkfleet validate`**: valida la config senza eseguire i check (target/url/dsn presenti, soglie coerenti, `checks` non vuoto); exit 1 su problemi, elenco leggibile. `engine.Validate`. _(v0.18.0)_
- [x] **CF-34 — Filtri finding**: `--only <check[,check]>`, `--min-severity ok|warn|bad|error`, `--target <glob>` sul comando `check`; i filtri valgono anche per `--exit-on-bad` e il `worst` JSON. `engine.Filter`. _(v0.17.0)_
- [x] **CF-35 — Retry/backoff su ERROR**: `retries`/`retry_backoff_ms` in config; il runner ritenta un check che produce finding ERROR (rete/handshake) con backoff esponenziale prima di riportarlo. `engine.RunWith(Options)`. _(v0.16.0)_

## M9 — Qualità (fase 2)

- [ ] **CF-36 — Fuzz dei parser**: `go test -fuzz` su m3u8 (stream), wire DNS, CSV HAProxy, `/jsz` NATS — i parser che leggono input esterno non fidato.
- [ ] **CF-37 — Suite d'integrazione opt-in**: harness con docker-compose (nats, haproxy, postgres, consul, redis…) dietro build tag/flag, fuori dai unit test; gira in CI separata, mai nei `go test ./...` di default.
