# Changelog

## 0.52.0

- Alert PagerDuty/Opsgenie (CF-28): nuovo `checkfleet alert --provider pagerduty|opsgenie --key-env <ENV>` — crea alert per i finding BAD/ERROR (dedup per `check/target`) e, con `--history`, risolve quelli rientrati rispetto al run precedente. Package `internal/alert` con `Plan` (trigger/resolve) e i payload PagerDuty (Events API v2) / Opsgenie testati; poster HTTP sottile. Chiave da env, mai in CLI/config.

## 0.51.1

- Rimossi i riferimenti aziendali dal progetto (sviluppato a titolo personale): contatto della licenza commerciale (`COMMERCIAL.md`) sull'email personale dell'autore (`allannava95@gmail.com`); neutralizzati gli host d'esempio in `checkfleet.example.yml` (dominio aziendale → `example.com`, DN LDAP, realm, service gRPC); ripuliti i riferimenti in `BACKLOG.md`, nel fixture di `internal/backlog`, nel commento di `internal/checks/nats` e il puntatore runbook in `CLAUDE.md`. Licenziante (`LICENSE`) e contatto coincidono con l'autore.

## 0.51.0

- Issue GitLab (CF-26): `report-issues --forge gitlab` riconcilia le issue su GitLab via `glab` (adapter `glIssueClient`: list/create/close+note, ensureLabel), con la stessa logica di reconcile di CF-7 (GitHub). Factory `issueClient(forge)` testata; `--forge github|gitlab` (default github). Il rispettivo CLI (`gh`/`glab`) dev'essere installato e autenticato.

## 0.50.1

- Docs: nuovo `COMMERCIAL.md` — come ottenere una licenza commerciale (cosa è già coperto dall'uso non-commerciale, quando serve la licenza, cosa concede, come richiederla via email/issue, dati utili per il preventivo). Collegato dalla sezione License del README. È un riepilogo di comodo: in caso di conflitto vale il testo di `LICENSE`.

## 0.50.0

- **Cambio licenza: da MIT a PolyForm Noncommercial 1.0.0** (source-available). Dal v0.50.0 in poi l'uso è libero solo per scopi **non commerciali** (personale, ricerca, istruzione, organizzazioni no-profit, enti pubblici); qualsiasi uso commerciale richiede una licenza separata dall'autore. Le release **fino al v0.49.0 restano sotto MIT** (il cambio non è retroattivo). Testo verbatim dalla fonte ufficiale con riga `Required Notice`. Aggiornati README (badge + sezione License), footer/badge del sito docs, about dell'app desktop (`main.go`/`wails.json`), CLAUDE/AGENTS. Non è consulenza legale.

## 0.49.0

- `--diff` (CF-51): con `--history <file>`, `checkfleet check … --diff` mostra solo cosa è cambiato rispetto al run precedente registrato — finding **new / resolved / worsened / improved** per check/target — invece della tabella completa. Utile per un cron che riporta solo i delta. `engine.DiffStatus` pura e testata; helper CLI `diffFromRecords`/`formatDiff` testati. **Chiude M13.**

## 0.48.0

- `--watch` (CF-50): `checkfleet check … --watch <interval>` riesegue i check a intervallo e ridisegna una vista live nel terminale (clear-screen + header + output text), Ctrl-C per fermare. Maintenance e filtri applicati a ogni tick. Helper `watchFrame` testato (loop I/O a parte).

## 0.47.0

- DX CLI (CF-54): nuovo `checkfleet explain [module]` — stampa cosa controlla un modulo e le soglie chiave (senza argomento lista i moduli); mappa guidata dal registry con test anti-drift. Nuovo `checkfleet completion <bash|zsh|fish>` — script di completamento per subcomandi, moduli (dopo `check`/`explain`) e formati `--output`. Testati.

## 0.46.1

- Fix `fuzz.yml`: espressione `fuzztime` con apici raddoppiati (`''60s''`) → "Invalid workflow file". Corretto nel literal valido dell'espressione GitHub (`'60s'`). Il workflow ora è caricabile.

## 0.46.0

- Finestre di manutenzione (CF-52): sezione `maintenance:` in config — finestre con glob `check`/`target` e range `from`/`to` (RFC3339). `action: mute` (default) elimina i finding nella finestra, `action: warn` declassa BAD/ERROR a WARN annotando ` [maintenance]`. La prima finestra attiva che matcha vince. `engine.ApplyMaintenance` testata; applicata al comando `check` (prima di `--exit-on-bad`) e a `serve`.

## 0.45.0

- Config dinamica (CF-53): i valori di `checkfleet.yml` supportano l'interpolazione `${VAR}`, `${VAR:-default}` e `${file:/path}` (secret da file, stile Docker/K8s), espansa prima del parse; `$${` per un `${` letterale. Un file secret mancante è errore. `engine.expandVars` testato. Tiene i segreti fuori dalla config restando compatibile coi campi `*_env` dei moduli.

## 0.44.0

- Output `discord` e `teams` (CF-27): `--output discord` invia un embed a un webhook Discord, `--output teams` una MessageCard a un incoming webhook Microsoft Teams — summary + problemi worst-first (cap 15), colore per worst status. URL da `--webhook-env` (mai in CLI/config), come Slack. Renderer `output.Discord`/`output.Teams` testati (JSON valido, titolo, problemi, cap, all-green).

## 0.43.0

- Output `html` (CF-29): `--output html` produce un report **statico autoconsistente** (CSS inline, nessuna risorsa esterna) col tema del sito — pill worst-status, tiles OK/WARN/BAD/ERROR, sezione "Needs attention" e tabella completa; messaggi HTML-escaped. Ideale come artifact di CI o allegato a un incident. Renderer `output.HTML` testato (struttura, summary, escaping). Con `--out-file` scrive su file atomico.

## 0.42.0

- Fuzz dei parser (CF-36, **chiude M9**): fuzz target `go test -fuzz` sui parser che leggono input esterno non fidato — `parseM3U8` (stream/HLS), `parseMessage` (DNS wire, parsing byte a mano con compression pointer), `parseCSV` (HAProxy stats), decode `/jsz` + `analyzeMeta` (NATS). White-box, in-package; i seed girano già come unit test in `go test ./...`. Nessun crasher trovato (~4.7M esecuzioni totali in locale, 15s/target).
- Nuovo workflow `fuzz.yml`: fuzza attivamente ogni target (matrice) — settimanale, `workflow_dispatch` (con `fuzztime` configurabile) e sulle PR che toccano un parser; carica gli eventuali crasher da `testdata/fuzz/` come artifact.

## 0.41.0

- Docs: nuova pagina **Desktop app** (`docs/desktop.md`) con demo dell'app GUI — screenshot dark+light (retina) del frontend reale e walkthrough della fleet view (toolbar, summary, tabella finding, filtri, export, stack, auto-refresh, tema), avvio con `CHECKFLEET_CONFIG`/`CHECKFLEET_AUTORUN`, download dalle release e build da sorgente. Aggiunta alla nav e alla home; CI/Development rinumerati.

## 0.40.0

- Guardrail English-output + sweep finale (CF-60, **chiude M15**):
  - Nuovo `scripts/check-english.sh` — fallisce se trova vocali accentate o parole italiane distintive nei `.go` di `cmd/`+`internal/`. Aggiunto come step di `ci.yml` (anti-regressione).
  - Il guardrail ha scovato italiano rimasto fuori da CF-58/59: tradotti i test di `engine` (filter/engine/stack/config), `history`, `issuesync`, `backlog`, `output/prometheus` e il tool `cmd/backlog-sync` (messaggi + body delle issue). Ora `go test ./...` e il guardrail sono verdi.
  - Sito docs e README: esempi di output portati in inglese (`want`/`expires in`/`N checks:`/`Needs attention`); neutralizzati gli host/realm aziendali rimasti negli esempi (dominio aziendale → `example.com`, `prod-cologno`→`prod`).
- Con CF-58/59/60 la migrazione i18n è completa: tutto l'output e i test del progetto sono in inglese (CHANGELOG escluso, per convenzione).

## 0.39.0

- Output in inglese — finding di **tutti i 18 moduli** (CF-59, M15): `certs, http, nats, haproxy, stream, patroni, consul, postgres, dns, redis, keycloak, tcp, tls, ntp, grpc, ldap, kafka, rabbitmq`. Tradotti i messaggi dei finding (reachability, soglie, lag, drift, ecc.) e aggiornati i test, incluse le asserzioni `strings.Contains` sul contenuto del messaggio. Neutralizzati alcuni realm/host aziendali nei test (keycloak). Con CF-58 (v0.38.0) l'intero output del CLI è ora in inglese. Chiude di fatto la migrazione lato CLI; resta il guardrail CF-60. CHANGELOG resta in italiano.

## 0.38.0

- Output in inglese — engine & CLI (CF-58, M15): tradotti `usage`, help dei flag, errori sistemici (`unknown module`, `module %q is not configured`, `no module selected`…), messaggi di `validate` (`… is valid ✅`, `N problem(s):`), i problemi di `engine.Validate` (`no target`, `has no url/dsn`, `should be >=`…), il finding di flapping, i messaggi di `serve`/`report-issues`, e i renderer `internal/output`: summary `N checks: …`, sezioni Markdown `Needs attention`/`All results` (header tabella `Status/Check/Target/Detail`), nota Slack `All green`/`…and N more problems`. Test aggiornati. I **messaggi dei finding dei moduli** restano da tradurre (CF-59, uno per release). CHANGELOG resta in italiano.

## 0.37.2

- Fix E2E desktop (`desktop-test.yml`): la webview si apriva ma lo screenshot era vuoto (1 colore → job rosso). Su Ubuntu 24.04 `libwebkit2gtk-4.1` è WebKitGTK ≥2.42, che di default usa il **DMABUF renderer**: sotto la GL software di Xvfb dipinge un frame nero. Aggiunto `WEBKIT_DISABLE_DMABUF_RENDERER=1` (+`GDK_BACKEND=x11`) per forzare il path software. La verifica ora **fa polling** dello screenshot (fino a 20 tentativi) invece di un singolo `sleep 3`, e dumpa `app.log` in caso di blank.

## 0.37.1

- Pianificata la migrazione dell'output in **inglese** (M15 · CF-58..60 nel BACKLOG): CF-58 engine & CLI, CF-59 messaggi dei finding per modulo (uno per release), CF-60 sweep & guardrail. Il desktop è già in inglese (v0.36.1); il CHANGELOG resta in italiano.
- CLAUDE.md: fissata la convenzione — codice, test e output user-facing in inglese; i nuovi moduli nascono già così.

## 0.37.0

- Output `webhook` (CF-49): `--output webhook` invia l'output JSON in POST a un URL generico (da `--webhook-env`), per qualsiasi sink che ingerisce JSON. Slack e webhook condividono l'helper `postJSON` (accetta 2xx). Telegram/syslog rimandati. **Chiude M12 (output & sink).**

## 0.36.1

- Desktop: stringhe user-facing portate in inglese (etichette UI, placeholder, header tabella, messaggi di stato/errore, titoli dei dialoghi, dati mock di anteprima). Coerente con la scelta di tenere codice e UI del desktop in inglese. Il CLI/engine resta in italiano per ora (le finding reali arrivano da lì).

## 0.36.0

- Dead-man's-switch (CF-48): `--ping-url-env <ENV>` pinga un URL (stile Healthchecks.io) a fine run — base URL su successo, `<url>/fail` se il worst è BAD/ERROR. Best-effort (non fa fallire il comando). Con cron rileva anche il caso 'checkfleet non ha girato'.

## 0.35.0

- Output `prometheus` (CF-47): `--output prometheus` emette il formato text-exposition (le stesse metriche di `serve`) per un run one-shot. Nuovo `--out-file` scrive l'output in modo atomico (temp+rename) su file — adatto al textfile collector di node_exporter; vale per ogni formato stampabile.

## 0.34.0

- Output `junit` (CF-46): `--output junit` produce un report XML JUnit — un testcase per finding, `<failure>` su BAD, `<error>` su ERROR, WARN passante con nota. Per il test tab della CI. Renderer `output.JUnit` testato.

## 0.33.0

- Test E2E dell'app desktop (nuovo job `e2e` in `desktop-test.yml`): builda il binario Wails reale, lo lancia headless sotto **Xvfb** con config seed + auto-run, e verifica che la **webview nativa** crei una finestra e renderizzi (screenshot non-blank via `xdotool`+ImageMagick, caricato come artifact). Esercita embed + runtime Wails + binding, non solo il frontend in browser.
- Nuova feature abilitante (utile anche come "apri con"): `App.StartupConfig()` — l'app si apre sulla config indicata da `CHECKFLEET_CONFIG` (fallback `./checkfleet.yml`) e, se `CHECKFLEET_AUTORUN=1`, esegue i check al lancio. Testata (`app_test.go`) e cablata nel frontend.
- I test desktop ora coprono tre livelli: **binding Go** (unit), **smoke frontend** (render in headless Chrome) ed **E2E** (app impacchettata reale).

## 0.32.1

- Homebrew: nuovo workflow `brew-test.yml` che verifica `brew install Allan-Nava/tap/checkfleet` end-to-end su macOS **Apple Silicon (macos-14) e Intel (macos-13)** — gira dopo ogni Release (via `workflow_run`, solo se la release è passata), a mano (`workflow_dispatch`, con assert opzionale della versione) e settimanale. Controlla: install dalla tap, `checkfleet version` reale (non `dev`), attributo `com.apple.quarantine` rimosso, e smoke di un check (`tcp`). `HOMEBREW_NO_REQUIRE_TAP_TRUST` per l'install headless su Homebrew 6+.
- Docs: `installation.md` e README aggiornati con la nota tap (form `brew tap` + trust di Homebrew 6+).

## 0.32.0

- Test dell'app desktop in CI (nuovo workflow `desktop-test.yml`, gira su push/PR che toccano `desktop/**`):
  - **Binding Go** (`desktop/app_test.go`): `RunChecks` end-to-end offline (check `tcp` verso un listener locale → OK, moduli/finding/summary corretti), errori di config, `ListStacks`, `DefaultConfigPath`, export Markdown/JSON.
  - **Smoke test frontend**: carica `frontend/dist/index.html` in headless Chrome (backend mock) e verifica che la vista fleet si renderizzi davvero (summary, worst=ERROR, badge di stato, tabella finding popolata).
- Fix (trovato dai test): `ListStacks` scambiava il file base `checkfleet.yml` per uno stack `"yml"`; ora richiede la forma `checkfleet.<stack>.<ext>`.

## 0.31.2

- Homebrew: tap `Allan-Nava/homebrew-tap` attivata. `skip_upload: "false"` in `.goreleaser.yaml` → a ogni tag `v*` goreleaser pubblica il cask sulla tap, quindi `brew install Allan-Nava/tap/checkfleet` funziona (repo tap + secret `HOMEBREW_TAP_GITHUB_TOKEN` già configurati). Il cask distribuisce il binario precompilato (darwin amd64/arm64) e rimuove l'attributo `com.apple.quarantine` all'installazione (binario non firmato). Solo i tag successivi all'attivazione portano il cask.

## 0.31.0

- Release: l'app desktop Wails viene allegata a **ogni** GitHub Release (tag `v*`). Il workflow `desktop.yml` builda per macOS (`.app` universale), Linux e Windows, aspetta che goreleaser abbia creato la release e vi carica gli eseguibili (`checkfleet-desktop_<versione>_<os>_<arch>.zip|.tar.gz`). Resta un workflow separato: se il build desktop fallisce, la release del CLI non si blocca. Eseguibile anche a mano via `workflow_dispatch` (carica gli artifact del workflow).

## 0.30.1

- Desktop: `desktop/go.mod` + `desktop/go.sum` risolti (dep indirette di Wails) così `wails build` gira out-of-the-box; `.gitignore` esteso a `build/darwin`, `build/windows` e al binario vagante. Build verificata in locale: `.app` universale (x86_64+arm64) con frontend embeddato e icona dal logo — conferma che la GUI è un binario nativo unico, non un servizio a parte.

## 0.30.0

- Modulo `kafka` (CF-43): health cluster via `franz-go`/`kadm`. Controller assente → BAD, broker sotto `expect_brokers` → WARN, partizioni under-replicated → BAD, lag dei consumer group in `groups` oltre `lag_warn`/`lag_crit` → WARN/BAD. TLS+SASL (plain/scram) opzionali, password da env. I/O dietro interfaccia: logica testata con fake (nessun broker reale nei test). Nuove dip: `github.com/twmb/franz-go` (+kadm).

## 0.29.0

- Modulo `ldap` (CF-42): connect + bind (anonimo o con credenziali da env) + search di sanity opzionale (≥ `min_entries` sotto `base_dn`). Bind fallito → BAD, connessione → ERROR. `ldaps`/StartTLS supportati. Accesso LDAP dietro interfaccia `session`: logica testata con fake; adattatore `go-ldap` sottile. Nuova dip: `github.com/go-ldap/ldap/v3`.

## 0.28.1

- CI integration: healthcheck HAProxy corretto (workflow `Integration` rosso). `bind *:8404` ascolta solo IPv4 ma `/etc/hosts` mappa `localhost` anche a `::1`; sui runner dove busybox `wget` preferisce IPv6 l'healthcheck colpiva `[::1]:8404` (nessun listener) e falliva sempre → `docker compose up --wait` in errore. Ora punta a `http://127.0.0.1:8404/stats;csv` (coerente col bind e col target del modulo). Gli altri servizi non sono affetti (nats/patroni/keycloak ascoltano dual-stack).

## 0.28.0

- Modulo `grpc` (CF-41): gRPC Health Checking Protocol (`grpc.health.v1.Health/Check`) su **HTTP/2 + TLS** con protobuf/framing gRPC scritti a mano — **zero dipendenze** (niente libreria gRPC; h2c plaintext non supportato). SERVING=OK, NOT_SERVING/SERVICE_UNKNOWN=BAD, UNKNOWN=WARN; grpc-status 12 (UNIMPLEMENTED)=WARN, 5 (NOT_FOUND)=BAD. Testato contro un finto server gRPC h2/TLS in-test.

## 0.27.0

- Modulo `rabbitmq` (CF-45): health via management HTTP API (zero-dip). Reachability+versione (`/api/overview`), nodi non-running o con memory/disk alarm → BAD (`/api/nodes`), profondità code oltre `queue_warn_depth`/`queue_crit_depth` → WARN/BAD e backlog senza consumer → WARN (`/api/queues`). Basic-auth con password da env. Testato con httptest.

## 0.26.1

- Desktop: i dati mock di anteprima usano placeholder neutri (`example.com`, host generici, `/home/ops/checkfleet.yml`) — rimossi i riferimenti a domini/host aziendali.

## 0.26.0

- **App desktop Wails** (M5, CF-15..18): nuovo frontend GUI in `desktop/` che riusa `internal/engine`/`internal/registry`/`internal/output` — il CLI resta la fonte di verità, la GUI è solo un altro frontend.
  - **Modulo Go separato** (`desktop/go.mod`, `replace => ../`): la toolchain web di Wails resta fuori dal modulo CLI, `go build/test ./...` e la CI del root non la tirano dentro (CF-15).
  - **Vista fleet** (CF-16): carica `checkfleet.yml`, esegue i check, summary (worst + tiles OK/WARN/BAD/ERROR + chip moduli) e tabella finding worst-first con badge colorati.
  - **Run & refresh** (CF-17): bottone Run, auto-refresh a intervallo, selettore stack (scopre `checkfleet.<stack>.yml`), filtri testo + min-severity, export Markdown/JSON via `internal/output` con file-dialog nativo.
  - **Packaging** (CF-18): icona da `docs/assets/logo.svg`, `wails.json`, `desktop/README.md` con i comandi build macOS/Linux, workflow `desktop.yml` **dispatch-only** separato dalla release CLI (goreleaser).
  - Frontend statico (HTML/CSS/JS, niente bundler), dark-first coerente col sito docs; apribile nel browser con dati mock per anteprima senza toolchain.
- Milestone GitHub per feature M5.1–M5.4 (CF-15..18).

## 0.25.0

- Suite d'integrazione opt-in (CF-37): stack `docker-compose.integration.yml` con servizi reali (redis, nats, consul, haproxy, postgres, patroni+etcd, keycloak) e `checkfleet.integration.yml` che li punta su `127.0.0.1`.
  - Test in `test/integration/` dietro build tag `integration`: `go test -tags integration ./test/integration/...`. **Fuori** dai unit test — `go test ./...` resta offline (server in-test) e non li esegue.
  - Contratto d'integrazione volutamente lasco: reachability (≥1 finding non-ERROR per modulo), non lo status esatto — quello resta coperto dai unit test.
  - Workflow CI separato `.github/workflows/integration.yml` (push/PR + `workflow_dispatch`): alza lo stack con `docker compose up --build --wait`, gira la suite e lo smoke `checkfleet check all`, poi `down -v`. Non tocca il job `test` di `ci.yml`.
  - Patroni: immagine single-node costruita in-compose (`deploy/integration/patroni/`) su base `postgres:16` + `patroni[etcd3]`; HAProxy con `deploy/integration/haproxy.cfg` (stats CSV su :8404). NATS standalone segnala BAD "no meta-leader" (atteso: un nodo singolo non è un cluster HA; exit-code-neutral).

## 0.24.0

- Modulo `ntp` (CF-40): offset dell'orologio via query SNTP a mano (UDP, zero dip); WARN/BAD oltre `offset_warn_ms`/`offset_crit_ms`, BAD se server non sincronizzato (stratum 0/≥16). Query isolata dietro funzione per test deterministici delle soglie.
- Refactor: registro dei moduli spostato in `internal/registry` (`Modules`/`Configured`), condiviso da CLI e (futura) app desktop — aggiungere un modulo ora si fa in un solo punto.

## 0.23.0

- Modulo `tls` (CF-39): TLS "profondo" che completa `certs`. Verifica catena vs trust store (BAD se non fidata/hostname mismatch), scadenza leaf (WARN/BAD), versione protocollo negoziata (< TLS 1.2 → WARN; si connette permissivo per poterlo segnalare). Zero-dip; testato con CA/leaf generati al volo. Etichette `[chain]`/`[expiry]`/`[protocol]`.

## 0.22.0

- Modulo `tcp` (CF-38): reachability TCP generica — connect (opz. TLS) + latenza, banner atteso opzionale (substring). Stdlib `net`, zero dip. Config `checks.tcp`, testato con listener in-test. Apre **M10**.

## 0.21.1

- Roadmap fase 3: nuove milestone e feature candidate nel BACKLOG (CF-38..57).
  - **M10 — Generici & protocollo**: `tcp`, `tls` profondo, `ntp`, `grpc`, `ldap`.
  - **M11 — Datastore & broker**: `kafka`, `mongodb`, `rabbitmq`.
  - **M12 — Output & sink**: `--output junit`, `--output prom-textfile`, dead-man switch (Healthchecks.io), sink generici (webhook/Telegram/syslog).
  - **M13 — Engine & UX**: `--watch`, `--diff` vs storico, finestre di manutenzione/mute, `${VAR}`+secret da file, completion & `explain`.
  - **M14 — Distribuzione & supply-chain**: immagine Docker multi-arch su GHCR, firma cosign + SBOM, govulncheck + golangci-lint in CI.
  - Al push `backlog-sync` apre le 20 nuove issue.

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
