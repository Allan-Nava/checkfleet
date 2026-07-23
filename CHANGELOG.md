# Changelog

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
