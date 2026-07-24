# CLAUDE.md — checkfleet

**checkfleet** (`github.com/Allan-Nava/checkfleet`): CLI Go open source di monitoring domain-specific — una flotta di check pluggabili in un solo binario (`checkfleet check all|certs|http --config checkfleet.yml`). Output text/markdown/JSON, target anche da inventory Ansible. Filosofia: NON rifare Prometheus/Grafana — fare i check che richiedono conoscenza di dominio e delegare grafici/alerting.

## Regole di lavoro (SEMPRE)

- **Ogni commit = release taggata `vX.Y.Z`**: nuova sezione in `CHANGELOG.md` (Keep a Changelog, in italiano) + `git tag -a vX.Y.Z -m "Release X.Y.Z"`. Bump `minor` per novità sostanziali (nuovi moduli/output), `patch` per fix. Senza chiederlo. **Esenti**: auto-commit su `.claude/settings.json` e commit `report:` CI.
- **MAI `git push`** — lo fa sempre l'utente. MAI `Co-Authored-By` nei commit.
- **Gate prima di chiudere**: `go vet ./...` + `go test ./...` verdi (stessi check della CI).
- **Ogni modulo nuovo = package in `internal/checks/<nome>`** che implementa `engine.Check`, config tipata in `engine/config.go`, wiring in `cmd/checkfleet/main.go`, **test con server/fixture locali** (mai rete esterna nei test, mai infrastruttura reale).
- **Exit code semantics**: 0 anche con finding WARN/BAD (il check che gira È un successo); ≠0 solo per errori sistemici (config illeggibile, modulo sconosciuto). `--exit-on-bad` per il gating CI. NON cambiare questa semantica.
- **Niente segreti** in config d'esempio, test, doc o output. I check non loggano mai credenziali.
- **Todo → `BACKLOG.md`** (sorgente unica, item con id stabile `CF-n`). Non sparpagliare TODO nei commenti.

## Comandi

```bash
go build -o checkfleet ./cmd/checkfleet
go test ./...            # unit + moduli contro server TLS/HTTP locali in-test
go vet ./...
./checkfleet check all --config checkfleet.example.yml
./checkfleet check certs --config checkfleet.yml --output markdown
```

## Architettura

- `internal/engine/` — contratto (`Check`, `Finding` con Status OK/WARN/BAD/ERROR, `Run` con timeout e sort worst-first, `Summarize`/`Worst`) + config YAML tipata con default (`LoadConfig`).
- `internal/output/` — renderer: `Text` (terminale), `Markdown` (stile report ops: summary, "Da guardare", tabella completa), `JSON` (con `worst` per il gating).
- `internal/checks/certs/` — scadenza TLS: dial con SNI e `InsecureSkipVerify` (vogliamo la scadenza anche se la chain non valida), soglie warn/crit in giorni, target espliciti + inventory Ansible, concorrenza con semaforo (16).
- `internal/checks/httpcheck/` — probe HTTP: status atteso, latenza max (WARN), substring body (BAD), errori di rete (ERROR).
- `internal/inventory/` — parser minimale inventory INI Ansible: host + `ansible_host`, sezioni `:vars`/`:children` ignorate, file o directory, dedup.
- `cmd/checkfleet/` — CLI a subcomandi (stdlib `flag`, niente cobra), `version` iniettata con `-ldflags "-X main.version=..."`.

## Trappole note / regole tecniche

- **`InsecureSkipVerify: true` nel modulo certs è VOLUTO** (leggere la scadenza anche con chain non valida localmente): non "sistemarlo".
- Lo stato ERROR significa "il check non è riuscito a misurare" (rete, handshake), non "target malato": non confonderlo con BAD.
- I test dei moduli creano server locali in-test (TLS con cert generati al volo a scadenza nota, `httptest`): ogni nuovo modulo deve fare lo stesso — un test che tocca internet o infra reale è un bug.
- Il sort dei finding è worst-first, stabile per check/target: l'ordine è API di fatto per chi parsa l'output text.
- Dipendenze: `gopkg.in/yaml.v3` (config) e `github.com/jackc/pgx/v5` (driver del modulo `postgres`, SQL indispensabile — unica eccezione motivata). Aggiungerne altre solo con forte motivazione. Moduli che parlano HTTP/JSON (nats, haproxy, consul, patroni, stream) NON devono introdurre driver: stdlib.
- Il campo `version` è iniettato dalla CI sui tag: non hardcodarlo.

## Puntatori

- Backlog: `BACKLOG.md` · CI: `.github/workflows/ci.yml` (vet+test su push/PR; tag `v*` → build multipiattaforma in release)
- Config d'esempio: `checkfleet.example.yml`
- Repo affini (famiglia Lens/tooling): `~/projects/github.com/ansible-vars-lens`, `nats-lens`, `nomad-lens` · Runbook di riferimento: `~/projects/hiway/devops_hiway/CLAUDE.md`
