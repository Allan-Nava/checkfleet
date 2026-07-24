# Changelog

## 0.21.0

- Modulo `keycloak` (CF-20): health via HTTP/JSON, zero-dip, nessuna credenziale.
  - Health endpoint (`health_url`, spesso sulla porta management) → UP=OK, DOWN=BAD, irraggiungibile=ERROR.
  - Per realm: discovery OIDC (`/realms/<realm>/.well-known/openid-configuration`) → token_endpoint presente=OK, 404/invalida=BAD, issuer non coerente con `/realms/<realm>`=WARN (misconfig proxy), irraggiungibile=ERROR.
  - Config `checks.keycloak` (`base_url`, `health_url`, `realms`); testato con httptest.
- CLI: `checkfleet check keycloak`. Docs e config d'esempio aggiornate.

## 0.20.0

- Modulo `redis`/`valkey` (CF-19): health via `INFO` con **client RESP minimale in-tree** (zero dipendenze), sola lettura.
  - Reachability + ruolo (WARN se `loading`); connect/PING/INFO falliti → ERROR.
  - Memoria: `used_memory` ≥ `mem_warn_pct`% di `maxmemory` → WARN.
  - Replica: `master_link_status` != up → BAD; offset lag oltre `lag_warn_bytes`/`lag_crit_bytes` → WARN/BAD.
  - Persistenza: ultimo bgsave RDB o scrittura AOF fallita → WARN.
  - TLS (`rediss`) e ACL opzionali; password da env (`password_env`), mai in config.
  - Config `checks.redis` (default `port: 6379`); target espliciti + inventory Ansible. Testato contro un finto server RESP in-test (nessuna infra reale).
- CLI: `checkfleet check redis`. Docs e config d'esempio aggiornate. Apre **M6**.

## 0.19.1

- README: header con logo e badge (release/CI/license/Go), sezioni allineate allo stato attuale.
  - Rimosso "in arrivo" ormai fatto (output Slack, exporter Prometheus); roadmap moduli aggiornata (redis/valkey, keycloak, mediamtx, s3, smtp, elasticsearch).
  - Aggiunti a Usage: `validate`, filtri finding (`--only`/`--min-severity`/`--target`), `--stack`; config d'esempio con `retries`/`retry_backoff_ms`.
- Nuovo asset `docs/assets/logo.png` (256px) per il README.

## 0.19.0

- Storico & flap detection (CF-32): `--history <file>` registra ogni run in un file **JSONL** (zero dipendenze) e aggiunge un finding `flap` (WARN) per ogni `check/target` che cambia stato ≥ `--flap-changes` volte nelle ultime `--flap-window` run. Package `internal/history` (Append/Recent/Flaps), testato. SQLite scartato per restare zero-dep. **Chiude M8 (engine & UX).**

## 0.18.0

- Comando `validate` (CF-33): valida la config senza eseguire i check — target/url/dsn presenti, soglie coerenti (warn vs crit), `checks` non vuoto. Exit 1 con elenco dei problemi. `engine.Validate`, testato. Utile in CI/pre-commit.

## 0.17.0

- Filtri finding (CF-34) sul comando `check`: `--only <check,...>`, `--min-severity ok|warn|bad|error`, `--target <glob>`. Si applicano all'output renderizzato e quindi anche a `--exit-on-bad` e al `worst` JSON. Funzione `engine.Filter` + `engine.ParseStatus`, testate.

## 0.16.0

- Retry/backoff su ERROR (CF-35): nuovi `retries` e `retry_backoff_ms` (top-level). Un check che produce finding ERROR (rete/handshake) viene ritentato con backoff esponenziale prima di riportarlo, riducendo i falsi ERROR transitori. Nuova `engine.RunWith(Options)`; `Run` resta come wrapper. Vale per `check`, `serve`, `report-issues`.

## 0.15.0

- Engine: `Run` esegue i check **in parallelo** (CF-31) — una goroutine per modulo, ciascuna col proprio timeout. Raccolta per-indice + sort stabile: output invariato e deterministico, ma wall-clock ≈ check più lento invece della somma. Test con `-race`.

## 0.14.1

- Roadmap fase 2: nuove milestone e feature candidate nel BACKLOG.
  - **M6 — Più moduli di dominio**: `redis`/`valkey`, `keycloak`, `mediamtx`, ingest RTMP/SRT, `s3`, `smtp`, `elasticsearch` (CF-19..25).
  - **M7 — Alerting & output**: issue GitLab, webhook Discord/Teams, PagerDuty/Opsgenie, report HTML, export OTLP (CF-26..30).
  - **M8 — Engine & UX**: check concorrenti, storico+flap/trend, `validate`, filtri finding, retry/backoff (CF-31..35).
  - **M9 — Qualità**: fuzz dei parser, suite integrazione opt-in con docker-compose (CF-36..37).
  - Priorità invariata: prossimo M5 (Wails), poi M6. Al push `backlog-sync` apre le 19 nuove issue.

## 0.14.0

- Docs (CF-10): ricetta CI **TeamCity** (build step con `--exit-on-bad` + service message) in `docs/ci.md`, accanto a GitHub Actions e cron. README con opzioni d'installazione (Homebrew/archivio) e link al sito. Chiude M4/Rilascio insieme al sito a tema custom (hero, ricerca, TOC, pagine per-modulo con esempi). GIF demo rimandata (non generabile qui; resta la demo testuale).

## 0.13.0

- Release con **goreleaser** (CF-9): nuovo `.goreleaser.yaml` + workflow `release.yml` sui tag `v*`. Archivi `tar.gz`/`zip` per linux/darwin/windows × amd64/arm64, `checksums.txt`, release notes dai commit, cask Homebrew.
- Rimosso il vecchio job `release` da `ci.yml` (build matrix manuale) per evitare release doppie.
- Tap Homebrew pronto ma **disattivo** (`skip_upload: "true"`): si attiva creando `Allan-Nava/homebrew-tap` + secret `HOMEBREW_TAP_GITHUB_TOKEN` e mettendo `skip_upload: "false"`. Validato con `goreleaser check` e `goreleaser release --snapshot`.
- Docs: installazione via archivio/Homebrew, sezione "Releasing" in development.

## 0.12.0

- Comando `report-issues` (CF-7): trasforma i finding BAD/ERROR in issue GitHub (una per `check/target`, dedup), aperte al fallimento e **chiuse automaticamente al rientro**. Idempotente, `--dry-run`, label `checkfleet-finding`; usa `gh`.
- Core di reconcile in `internal/issuesync` (interfaccia tracker + logica pura), testato con client finto; adattatore `gh` nel CLI. GitLab pluggabile in futuro (stessa interfaccia). **Chiude M4 (output & integrazione).**

## 0.11.0

- Config multi-stack (CF-8): flag `--stack <name>` (per `check` e `serve`) sovrappone `checkfleet.<stack>.yml` alla base. Merge per modulo (un modulo nello stack sostituisce quello base e riprende i suoi default), `timeout_seconds` sovrascritto solo se impostato.
- Refactor `LoadConfig` in `parseConfig` + `applyDefaults` + `overlay`; nuove `LoadConfigStack`/`StackPath`. Test dedicati (overlay, ereditarietà, path, errori).

## 0.10.0

- Comando `serve` (CF-6): modalità exporter Prometheus. `checkfleet serve --listen :9876 --interval 60s` riesegue i check a intervallo ed espone su `/metrics`: `checkfleet_finding_status{check,target}` (severity 0-3, worst per coppia), `checkfleet_findings_total{status}`, `checkfleet_worst_status`, durata e timestamp dell'ultimo run.
- Renderer `output.Prometheus` testato (formato, dedup-worst su serie duplicate, escaping label).
- Refactor: registry moduli unico (`modules()`) condiviso da `check` e `serve`.

## 0.9.0

- Output `slack` (CF-5): `--output slack` invia un messaggio Block Kit a un webhook Slack (header + summary + problemi worst-first, cap 20). URL del webhook da env (`--webhook-env`, default `SLACK_WEBHOOK`), mai in CLI/config. Renderer `output.Slack` testato (JSON valido, cap); POST thin nel CLI.

## 0.8.0

- Modulo `dns` (CF-13): health risoluzione DNS con **client DNS minimale in-tree** (zero dipendenze) — query a resolver specifici, TTL e serial SOA.
  - Nome che nessun resolver risolve → ERROR; nessun record del tipo richiesto → BAD.
  - Drift vs `expect` (per SOA confronta il serial) → BAD.
  - Consistenza tra resolver: risposte divergenti o serial SOA diversi (propagazione) → WARN; resolver che non risponde mentre altri sì → WARN.
  - TTL sotto `min_ttl_seconds` → WARN. Tipi: A, AAAA, CNAME, NS, TXT, SOA; resolver di default da `/etc/resolv.conf`.
  - Codec wire testato con round-trip; logica testata con `query` finto (nessuna rete nei test).
- CLI: `checkfleet check dns`. Docs e config d'esempio aggiornate.
- CF-14 (endpoint/disk) chiuso come **deciso-non-fare**: disco/host si delegano a node_exporter + alerting (coerente con "no agent" e "non rifare Prometheus"). **M3 = solo dns.**

## 0.7.1

- Docs site: nuovo tema custom (layout + SCSS in-repo), abbandonato `just-the-docs`.
  - Dark-first con toggle chiaro/scuro (preferenza salvata), palette emerald/slate.
  - Home "landing": hero con demo terminale, griglia feature, quickstart.
  - Sidebar di navigazione, TOC "on this page" con scroll-spy, ricerca client-side (`search.json`), paginazione prev/next, syntax highlighting Rouge brandizzato.
  - SEO/OpenGraph via `jekyll-seo-tag`; permalink "pretty" (`/installation/`).
- Nuovo logo: monogramma "cf"/check in emerald/slate — `docs/assets/logo.svg`, favicon SVG + fallback PNG (32px, apple-touch 180px).
- Gemfile docs: rimosso `just-the-docs`, aggiunti `jekyll-seo-tag` e `webrick`.

## 0.7.0

- Modulo `postgres` (CF-11): health PostgreSQL via SQL di sola lettura (driver `pgx`), mai DDL/scritture.
  - Reachability con ruolo (primary/replica); connessione/query fallita → ERROR.
  - Wraparound: `age(datfrozenxid)` oltre `wraparound_warn_age`/`wraparound_crit_age` → WARN/BAD.
  - Connessioni oltre `conn_warn_pct`% di `max_connections` → WARN.
  - Replication slot inattivi (WAL trattenuto) oltre `slot_warn_bytes`/`slot_crit_bytes` → WARN/BAD.
  - Replica lag (solo primary, `pg_stat_replication`) oltre `lag_warn_bytes`/`lag_crit_bytes` → WARN/BAD.
  - Accesso DB astratto dietro interfaccia: logica dei finding testata con DB finto (nessuna infra reale nei test). Password da env (`password_env`), mai in config.
- Nuova dipendenza: `github.com/jackc/pgx/v5` (motivata: il modulo postgres richiede SQL). CLAUDE.md aggiornato.
- CLI: `checkfleet check postgres`. Docs e config d'esempio aggiornate. **Chiude M2 (data layer)**.

## 0.6.0

- Modulo `consul` (CF-12): health cluster Consul via HTTP API, sola lettura.
  - Leader raft assente → BAD; peer sotto quorum → BAD, sotto l'atteso ma con quorum → WARN.
  - Health check in `critical` → BAD, in `warning` → WARN (etichetta `service@node`).
  - Chiavi KV mancanti (`kv_keys`) → BAD. ACL token opzionale da env (`token_env`), mai in config.
  - Config `checks.consul` (default `port: 8500`); target espliciti + inventory Ansible.
- CLI: `checkfleet check consul`. Docs e config d'esempio aggiornate.

## 0.5.0

- Modulo `patroni` (CF-4): health cluster PostgreSQL gestito da Patroni via REST API (`/cluster`), sola lettura.
  - Leader: assente → BAD (failover/quorum), più di uno → WARN (split-brain), uno → OK.
  - Replica: stato non running/streaming → WARN/BAD; lag oltre `lag_warn_bytes`/`lag_crit_bytes` → WARN/BAD (default 32/128 MiB); lag `unknown` → OK con nota.
  - Timeline replica diversa dal leader → WARN.
  - Config `checks.patroni` (default `port: 8008`); target espliciti + inventory Ansible.
- CLI: `checkfleet check patroni`. Docs e config d'esempio aggiornate.

## 0.4.2

- Automazione backlog ↔ issue: `BACKLOG.md` resta sorgente unica; ogni `CF-n` diventa una issue GitHub (label `backlog`, milestone per sezione M1–M5/Rilascio).
  - `internal/backlog`: parser di `BACKLOG.md` in item (id, titolo, milestone, done), con test.
  - `cmd/backlog-sync`: crea/chiude/riapre le issue in modo idempotente (match per prefisso `CF-n`), con `-dry-run`; usa `gh`.
  - Workflow `.github/workflows/backlog-sync.yml`: sync a ogni push su `main` che tocca il backlog + `workflow_dispatch`.
  - Creazione iniziale: 18 issue (15 aperte + CF-1/2/3 chiuse come completate).

## 0.4.1

- Docs: tema GitHub Pages passato da `cayman` a **just-the-docs** (sidebar di navigazione + ricerca full-text). Build via Gemfile + `ruby/setup-ruby` (Jekyll 4); `jekyll-relative-links` mantiene i link `.md` interni. Ordinamento pagine con `nav_order`.
- Audit + test (`internal/engine` da 0% a ~98%): `LoadConfig` (default e valori espliciti di tutti i moduli, moduli assenti = nil, errori), `Run` (sort worst-first stabile, timeout), `Worst`/`Summarize`. Suite verde con `-race`; `gofmt`/`vet` puliti. Nessun bug rilevato nel contratto.

## 0.4.0

- Modulo `stream` (CF-3): health HLS/DASH dai manifest (mai i segmenti media).
  - Manifest irraggiungibile → ERROR; manifest non valido (`.m3u8`/`.mpd`) → BAD.
  - Ladder: con `min_variants`, meno renditions dell'atteso → WARN, nessuna → BAD.
  - Freschezza live-edge (`live: true`) via HLS `#EXT-X-PROGRAM-DATE-TIME` (avanzato per durata segmenti) o DASH `publishTime` → WARN/BAD oltre `max_age_warn_seconds`/`max_age_crit_seconds` (default 30/60).
  - Live atteso ma VOD (`#EXT-X-ENDLIST` o MPD statico) → WARN; freschezza non misurabile senza timestamp → WARN (niente falsi OK).
  - Per un master HLS live, fetcha la variante a banda più alta per misurare il live-edge.
  - Config `checks.stream` con target multipli (`url`, `name`, `min_variants`, `live`, soglie età).
- CLI: `checkfleet check stream`. Docs e config d'esempio aggiornate.

## 0.3.0

- Modulo `haproxy` (CF-2): health backend/server via CSV stats HTTP (endpoint `;csv`), sola lettura.
  - Server DOWN → BAD; MAINT/DRAIN/NOLB → WARN; backend senza server disponibili → BAD.
  - Saturazione sessioni opzionale (`session_warn_pct`, `scur/slim`) → WARN.
  - Basic-auth opzionale con password da variabile d'ambiente (`auth_user` + `auth_pass_env`), mai in config.
  - Config `checks.haproxy` (default `port: 8404`, `path: /stats;csv`); target espliciti + inventory Ansible.
- CLI: `checkfleet check haproxy`. Docs e config d'esempio aggiornate.

## 0.2.1

- Fix rilascio CI: `.gitignore` aveva il pattern `checkfleet` non ancorato, che ignorava anche la directory `cmd/checkfleet/` — `cmd/checkfleet/main.go` non era mai stato committato. Il job `test` (`go build ./...`) non se ne accorgeva (globbing), ma il job `release` (`go build ./cmd/checkfleet`) falliva con *directory not found*. Ora il pattern è `/checkfleet` (solo il binario in root) e il sorgente della CLI è tracciato.

## 0.2.0

- Modulo `nats` (CF-1): health cluster NATS JetStream via endpoint di monitoring (`/varz`, `/jsz?meta=1`), sola lettura.
  - Reachability + versione per nodo; **versioni miste** nel cluster → WARN.
  - **Meta-leader**: assente → BAD, incoerente tra i nodi → WARN, diverso da `expect_meta_leader` → WARN.
  - **Peer**: OFFLINE → BAD, non current → WARN; **lag** raft oltre `lag_warn`/`lag_crit` → WARN/BAD.
  - **Ghost/missing peer** con `expect_peers`: membro inatteso → WARN, atteso assente → BAD.
  - Config `checks.nats` (default `port: 8222`, `lag_warn: 100`, `lag_crit: 1000`); target espliciti + inventory Ansible.
- CLI: `checkfleet check nats`. Docs e config d'esempio aggiornate.

## 0.1.2

- Docs: sito d'uso in `docs/` (installazione, configurazione, uso, moduli, output, CI, sviluppo) servito via GitHub Pages.
- CI: workflow `Pages` (`.github/workflows/pages.yml`) che builda `docs/` con Jekyll e pubblica su GitHub Pages.
- README: link al sito documentazione (`allan-nava.github.io/checkfleet`).

## 0.1.1

- Backlog: roadmap riorganizzata a milestone (M1 rete/delivery → M2 data layer → M3 piattaforma/host → M4 output → M5 app desktop).
- Nuovi item: `postgres` (CF-11), `consul` (CF-12), `dns` (CF-13), `endpoint`/`disk` (CF-14).
- Pianificata app desktop **Wails** (CF-15..CF-18): frontend che riusa `internal/engine`/`internal/output`, binario separato dalla CLI.

## 0.1.0

- Engine: contratto `Check`/`Finding` (OK/WARN/BAD/ERROR), runner con timeout e ordinamento worst-first, config YAML con default.
- Modulo `certs`: scadenza TLS con soglie warn/crit, target espliciti + inventory Ansible INI, probe concorrenti.
- Modulo `http`: status atteso, latenza massima (WARN), substring nel body.
- Output: text (terminale), markdown (report ops: summary, "Da guardare", tabella completa), JSON con `worst`.
- CLI: `checkfleet check <all|certs|http> --config --output --exit-on-bad`, `checkfleet version`.
- Exit code 0 anche su WARN/BAD (gating opzionale con `--exit-on-bad`).
