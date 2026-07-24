# Backlog — checkfleet

Sorgente unica dei todo. Id stabili `CF-n`; spuntare, non cancellare.

> **Sync automatico issue**: questo file è la fonte di verità. Ogni `CF-n` diventa una issue GitHub (label `backlog`, milestone per sezione) via `cmd/backlog-sync` + workflow `.github/workflows/backlog-sync.yml`. Spuntare un item (`[x]`) chiude la issue al prossimo push; toglierlo la riapre. Idempotente. Non aprire/chiudere le issue a mano: edita qui.

Roadmap a milestone. **Fase 1 (completa)**: cosa monitorare (M1→M3), come consegnarlo (M4), rilascio. **Fase 2**: usarlo meglio (M5 app desktop), più domini (M6), più alerting (M7), engine più solido (M8), qualità (M9). **Fase 3**: check generici/protocollo (M10), datastore & broker (M11), output & sink (M12), engine & UX (M13), distribuzione & supply-chain (M14). Le versioni sono indicative: ogni modulo/output è comunque una release taggata a sé. **M5 completa** (app desktop Wails, v0.24.0). **In corso:** M6 (redis ✅, keycloak ✅).

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

- [x] **CF-15 — Scaffold Wails**: progetto Wails in `desktop/` (**modulo Go separato**, `replace => ../`) che riusa `internal/engine`/`internal/registry`/`internal/output` senza duplicare logica; versione via ldflags. La CI del CLI non dipende dalla toolchain web (il modulo `desktop/` è escluso da `go ./...`). Registry moduli estratto in `internal/registry` (unica fonte, condivisa CLI+GUI). _(v0.24.0)_
- [x] **CF-16 — Vista fleet**: carica `checkfleet.yml`, esegue i check, mostra summary (worst + tiles OK/WARN/BAD/ERROR + chip moduli) e tabella finding con lo stesso sort worst-first; badge colorati per status. _(v0.24.0)_
- [x] **CF-17 — Run & refresh**: bottone Run, auto-refresh a intervallo, selettore stack (scopre `checkfleet.<stack>.yml`), filtri (testo + min-severity), export markdown/JSON via `internal/output` con file-dialog nativo. _(v0.24.0)_
- [x] **CF-18 — Packaging desktop**: icona da `docs/assets/logo.svg`, `wails.json`, `desktop/README.md` con i comandi `wails build` macOS/Linux, e workflow `desktop.yml` **dispatch-only** separato dalla release CLI (CF-9). _(v0.24.0; firma code-sign da configurare con i certificati)_

## Rilascio

- [x] **CF-9 — goreleaser** (linux/darwin/windows, amd64/arm64) + archivi + checksums + Homebrew cask. Tap pronto ma disattivo (`skip_upload`) finché non si crea `Allan-Nava/homebrew-tap` + secret. Validato con `goreleaser check` + `--snapshot`. _(v0.13.0)_
- [x] **CF-10 — Docs sito**: sito con tema custom (hero, ricerca, TOC, pagine per-modulo con esempi config), ricette CI GitHub Actions + TeamCity + cron, README ricco. GIF demo rimandata (asset binario, non generabile qui; c'è la demo testuale). _(v0.14.0)_

## M6 — Più moduli di dominio (fase 2)

- [x] **CF-19 — Modulo `redis`/`valkey`**: reachability (`PING`)+`INFO` con **client RESP minimale in-tree** (zero dip), uso memoria vs `maxmemory`, replica (link up/down, offset lag), persistenza RDB/AOF, loading. TLS + ACL (password da env). _(v0.20.0)_
- [x] **CF-20 — Modulo `keycloak`**: health endpoint UP, discovery OIDC per realm (token_endpoint presente, issuer coerente con `/realms/<realm>`). HTTP/JSON zero-dip, nessuna credenziale. _(v0.21.0; versione via admin rimandata — richiede auth)_
- [ ] **CF-21 — Modulo `mediamtx`**: API di mediamtx — path attivi, reader/publisher per path, path attesi presenti, ingest fermi. Codifica l'uso hiway (KV_mediamtx nel runbook NATS).
- [ ] **CF-22 — Modulo `ingest` (RTMP/SRT)**: l'endpoint di ingest accetta connessioni (handshake TCP/RTMP, o SRT), latenza. Segnale "lo streamer riesce a pubblicare?".
- [ ] **CF-23 — Modulo `s3`/object storage**: bucket raggiungibile, oggetto sentinella presente e fresco (last-modified sotto soglia), spazio/quota se esposta. Credenziali da env.
- [ ] **CF-24 — Modulo `smtp`**: il relay accetta connessioni, STARTTLS ok, cert del relay non scaduto, banner atteso. Nessun invio reale.
- [ ] **CF-25 — Modulo `elasticsearch`/`opensearch`**: `_cluster/health` (green/yellow/red), shard unassigned, nodi attesi presenti, disk watermark.

## M7 — Alerting & output (fase 2)

- [ ] **CF-26 — Issue GitLab**: implementa `issuesync.Client` per GitLab (via `glab`/API), stessa logica di CF-7. Selezione forge in config/flag.
- [x] **CF-27 — Webhook Discord/Teams**: `--output discord` (embed) e `--output teams` (MessageCard) — summary + problemi worst-first (cap), colore per worst status; URL da `--webhook-env` (mai in CLI). Renderer `output.Discord`/`output.Teams` testati; helper `postRendered` condiviso. _(v0.44.0)_
- [ ] **CF-28 — Alert PagerDuty/Opsgenie**: crea/risolve alert per finding BAD/ERROR (dedup per check+target), chiave di routing da env.
- [x] **CF-29 — Report HTML**: `--output html` — report statico autoconsistente (summary + tiles, "Needs attention", tabella completa), CSS inline, tema coerente col sito, messaggi HTML-escaped. Renderer `output.HTML` testato. _(v0.43.0)_
- [ ] **CF-30 — Export OTLP**: esporta i finding come metriche/eventi OpenTelemetry (OTLP), per chi non usa lo scrape Prometheus.

## M8 — Engine & UX (fase 2)

- [x] **CF-31 — Check concorrenti**: il runner esegue i moduli in parallelo (goroutine per check, timeout per-check), raccolta per-indice + sort stabile → output deterministico e wall-clock ≈ check più lento. _(v0.15.0)_
- [x] **CF-32 — Storico & flap/trend**: persistenza dei run in **JSONL** (zero dip, scelto vs SQLite); `--history <file>` registra il run e aggiunge finding `flap` (WARN) per i target che cambiano stato ≥ soglia nella finestra recente. Package `internal/history`. _(v0.19.0; SQLite scartato per la filosofia zero-dep)_
- [x] **CF-33 — `checkfleet validate`**: valida la config senza eseguire i check (target/url/dsn presenti, soglie coerenti, `checks` non vuoto); exit 1 su problemi, elenco leggibile. `engine.Validate`. _(v0.18.0)_
- [x] **CF-34 — Filtri finding**: `--only <check[,check]>`, `--min-severity ok|warn|bad|error`, `--target <glob>` sul comando `check`; i filtri valgono anche per `--exit-on-bad` e il `worst` JSON. `engine.Filter`. _(v0.17.0)_
- [x] **CF-35 — Retry/backoff su ERROR**: `retries`/`retry_backoff_ms` in config; il runner ritenta un check che produce finding ERROR (rete/handshake) con backoff esponenziale prima di riportarlo. `engine.RunWith(Options)`. _(v0.16.0)_

## M9 — Qualità (fase 2)

- [x] **CF-36 — Fuzz dei parser**: `go test -fuzz` su m3u8 (stream), wire DNS, CSV HAProxy, `/jsz` NATS — i parser che leggono input esterno non fidato. Fuzz target white-box in ciascun package; i seed girano nei unit test, il workflow `fuzz.yml` fuzza attivamente (schedule/dispatch/PR). _(v0.42.0)_
- [x] **CF-37 — Suite d'integrazione opt-in**: harness con docker-compose (nats, haproxy, postgres, consul, redis, patroni+etcd, keycloak) dietro build tag `integration` in `test/integration/`, fuori dai unit test; gira nel workflow separato `integration.yml`, mai nei `go test ./...` di default. _(v0.25.0)_

## M10 — Check generici & protocollo (fase 3)

- [x] **CF-38 — Modulo `tcp`**: connect a `host:port` (opz. TLS) + banner opzionale (substring), latenza. Reachability generica; stdlib `net`, zero dip. _(v0.22.0)_
- [x] **CF-39 — Modulo `tls`** (profondo): validità della catena, scadenza di ogni cert, protocolli/cipher deboli (TLS<1.2), hostname mismatch, OCSP se disponibile. Completa `certs` (che fa solo scadenza leaf). Stdlib `crypto/tls`.
- [x] **CF-40 — Modulo `ntp`**: offset dell'orologio oltre soglia, stratum, root dispersion. Query NTP a mano (UDP 123), zero dip. Il drift rompe TLS e token JWT.
- [x] **CF-41 — Modulo `grpc`**: gRPC Health Checking Protocol via HTTP/2+TLS con protobuf a mano (zero dip; h2c plaintext non supportato). SERVING/NOT_SERVING/UNIMPLEMENTED gestiti. _(v0.28.0)_
- [x] **CF-42 — Modulo `ldap`**: bind (anonimo o con credenziali da env) + search di sanity su una base DN. Valutare dip (`go-ldap`) vs protocollo a mano.

## M11 — Datastore & broker (fase 3)

- [x] **CF-43 — Modulo `kafka`**: broker raggiungibili, controller presente, under-replicated partitions, lag dei consumer group attesi. Valutare dip (`franz-go`/`sarama`) — protocollo Kafka non banale a mano.
- [ ] **CF-44 — Modulo `mongodb`**: `replSetGetStatus` (primary presente, membri health, lag), `serverStatus` connessioni. Valutare dip (driver mongo) vs wire protocol.
- [x] **CF-45 — Modulo `rabbitmq`**: management HTTP API — profondità code oltre soglia, backlog senza consumer, nodi non-running/alarm. HTTP/JSON, zero dip. _(v0.27.0)_

## M12 — Output & sink (fase 3)

- [x] **CF-46 — `--output junit`**: report XML JUnit (un testcase per finding, failure su BAD/ERROR) per il test tab di CI (TeamCity/GitHub Actions).
- [x] **CF-47 — `--output prometheus` + `--out-file`**: scrive le metriche nel formato del textfile collector di node_exporter (one-shot da cron), alternativa a `serve` per host senza server.
- [x] **CF-48 — Dead-man's-switch**: ping a Healthchecks.io (o URL configurabile) a fine run — success/fail in base al `worst`. Rileva anche il caso "checkfleet non ha girato".
- [x] **CF-49 — Sink generici**: `--output webhook` (POST JSON a URL da env). Telegram/syslog rimandati. _(v0.37.0)_

## M13 — Engine & UX (fase 3)

- [x] **CF-50 — `--watch`**: `--watch <interval>` riesegue i check a intervallo e ridisegna una vista text live nel terminale (clear + header + output), Ctrl-C per fermare; maintenance e filtri applicati a ogni tick. Helper `watchFrame` testato. _(v0.48.0)_
- [x] **CF-51 — `--diff`**: con `--history`, `--diff` stampa solo i cambi vs il run precedente — new/resolved/worsened/improved per check/target. `engine.DiffStatus` (pura, testata) + helper CLI `diffFromRecords`/`formatDiff`. _(v0.49.0)_
- [x] **CF-52 — Finestre di manutenzione / mute**: `maintenance:` in config — finestre con glob `check`/`target` + range `from`/`to` (RFC3339); `action: mute` (drop, default) o `warn` (cap BAD/ERROR a WARN + nota `[maintenance]`). `engine.ApplyMaintenance` testata; applicata a `check` (prima di `--exit-on-bad`) e `serve`. _(v0.46.0)_
- [x] **CF-53 — Config dinamica**: interpolazione `${VAR}`, `${VAR:-default}` e `${file:/path}` (secret da file, stile Docker/K8s) nei valori di config, espansa prima del parse; `$${` per `${` letterale; file secret mancante = errore. Testata. (Scelto `${file:…}` come meccanismo generale invece del per-campo `*_file`.) _(v0.45.0)_
- [x] **CF-54 — DX CLI**: `checkfleet explain [module]` (cosa controlla + soglie, guidato dal registry con test anti-drift) e `checkfleet completion <bash|zsh|fish>` (script che completano subcomandi, moduli e `--output`). _(v0.47.0)_

## M14 — Distribuzione & supply-chain (fase 3)

- [ ] **CF-55 — Immagine Docker**: immagine multi-arch (linux/amd64+arm64) pubblicata su GHCR via goreleaser, con l'exporter come entrypoint. Utile in k8s/nomad.
- [ ] **CF-56 — Firma & SBOM**: firma delle release con cosign (keyless) + SBOM (goreleaser). Provenienza verificabile.
- [ ] **CF-57 — Lint & vuln in CI**: `govulncheck` e `golangci-lint` come gate in CI, accanto a vet/test.

## M15 — Output in inglese (i18n) (fase 3)

Direzione stabilita: **tutto l'output user-facing del software è in inglese**. Il desktop è già stato convertito (v0.36.1). Restano CLI + engine + moduli. La convenzione è fissata in `CLAUDE.md` (i nuovi moduli nascono già in inglese). Il **CHANGELOG resta in italiano** (regola Keep a Changelog di progetto); le docs/sito sono già in inglese.

- [x] **CF-58 — Engine & CLI in inglese**: messaggi top-level e `usage`, errori sistemici (config illeggibile, modulo sconosciuto/non configurato), `validate`, help dei flag, e i renderer di `internal/output` (summary `N checks: …`, sezione "Needs attention"/"All results" del Markdown, note Slack). Test aggiornati. _(v0.38.0)_
- [x] **CF-59 — Finding dei moduli in inglese**: convertiti i messaggi di **tutti** i 18 moduli check — `certs, http, nats, haproxy, stream, patroni, consul, postgres, dns, redis, keycloak, tcp, tls, ntp, grpc, ldap, kafka, rabbitmq` — con i relativi test (asserzioni sul messaggio incluse). Realm/host aziendali nei test neutralizzati. _(v0.39.0)_
- [x] **CF-60 — Sweep & guardrail**: script `scripts/check-english.sh` (vocali accentate + wordlist italiana nei `.go` di `cmd/`+`internal/`) come step della CI (`ci.yml`). Ha scovato e fatto tradurre anche i test residui (`engine`, `history`, `issuesync`, `backlog`, `prometheus`) e il tool `backlog-sync`. Esempi di output nel sito docs e README portati in inglese; convenzione già in `CLAUDE.md`. **Chiude M15.** _(v0.40.0)_
