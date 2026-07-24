# AGENTS.md — checkfleet

**checkfleet**: CLI Go source-available (PolyForm Noncommercial 1.0.0) di monitoring domain-specific — check pluggabili in un solo binario (`checkfleet check all|certs|http`), output text/markdown/JSON, target anche da inventory Ansible.

Questo file definisce le regole operative per gli agent (Copilot, Claude, altri tool AI) quando lavorano in questo repository.

## Regole di lavoro (SEMPRE)

- **Ogni commit = release taggata `vX.Y.Z`**: nuova sezione in `CHANGELOG.md` (Keep a Changelog, in italiano) + `git tag -a vX.Y.Z -m "Release X.Y.Z"`. Bump `minor` per nuovi moduli/output, `patch` per fix. **Esenti**: auto-commit su `.claude/settings.json` e commit `report:` CI.
- **MAI `git push`**: lo fa sempre l'utente. MAI `Co-Authored-By` nei commit.
- **Gate prima di chiudere**: `go vet ./...` + `go test ./...` verdi.
- **Ogni modulo nuovo** = package `internal/checks/<nome>` che implementa `engine.Check` + config tipata + wiring in main + **test con server/fixture locali** (mai rete esterna o infra reale nei test).
- **Exit code**: 0 anche con WARN/BAD; !=0 solo per errori sistemici; `--exit-on-bad` per il gating. Non cambiare questa semantica.
- **Niente segreti** in config d'esempio, test, doc, output.
- **Todo -> `BACKLOG.md`** (id stabili `CF-n`), niente TODO sparsi.

## Comandi

- `go build -o checkfleet ./cmd/checkfleet` - `go test ./...` - `go vet ./...`
- Smoke: `./checkfleet check all --config checkfleet.example.yml`

## Trappole note

- `InsecureSkipVerify: true` nel modulo certs e' VOLUTO (serve la scadenza anche con chain non valida): non rimuoverlo.
- ERROR = "il check non ha potuto misurare"; BAD = "il target sta male". Non confonderli.
- Test sempre contro server locali creati in-test (TLS con cert a scadenza nota, httptest).
- Ordine finding worst-first stabile: e' API di fatto per chi parsa l'output.
- Unica dipendenza: `gopkg.in/yaml.v3`.
- `version` iniettata con `-ldflags` dalla CI: non hardcodare.

## Puntatori

- Backlog: `BACKLOG.md` - CI: `.github/workflows/ci.yml` - Esempio config: `checkfleet.example.yml`
