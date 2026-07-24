# Changelog

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
