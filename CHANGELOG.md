# Changelog

## 0.114.1

- Desktop (fix CI): il job `frontend-lint` falliva su `html-validate` con due `prefer-native-element` introdotti dal lavoro di accessibilità (CF-103) — `role="progressbar"` sulla barra di caricamento e `role="listbox"` sulla lista della command-palette. Aggiunte due **direttive inline** `html-validate-disable-next`, ognuna con la sua motivazione sul posto: un `<progress>` indeterminato non può ospitare lo sliver animato custom (con `appearance:none` non resta un box interno da animare), e la palette è un **combobox ARIA** con lista filtrata mentre digiti e focus che resta nell'input via `aria-activedescendant` — cose che un `<select>` nativo non sa fare. Nessun cambiamento visivo né di comportamento.
- Nota tecnica: la prima strada tentata era configurare `prefer-native-element` con un `mapping` nel `.htmlvalidate.json`, ma quell'opzione **sostituisce** la mappa di default invece di estenderla — avrebbe silenziosamente spento la regola anche per tutti gli altri ruoli (verificato: con il mapping, un `role="checkbox"` di prova smetteva di essere segnalato). Le direttive inline lasciano la regola pienamente attiva ovunque.

## 0.114.0

- Desktop (CF-106, M27 — "Send to…"): nuovo controllo **Send…** nella barra dei finding che inoltra il run corrente a **Slack / Discord / Teams / webhook** riusando i renderer di `internal/output` — nessuna logica duplicata. L'URL di destinazione arriva **solo da una variabile d'ambiente** (`SLACK_WEBHOOK`, `DISCORD_WEBHOOK`, `TEAMS_WEBHOOK`, `CHECKFLEET_WEBHOOK`): non si inserisce mai nella UI, quindi nessun segreto vive nell'app. L'esito torna via toast (inviato / non configurato con il nome dell'env da settare / errore). Binding `App.Send(target)` + `App.SendTargets`, con TDD via `httptest` (`TestSend`: nessun run, env vuoto, target ignoto, invio ok con payload ricevuto). Docs (`docs/desktop.md`, `desktop/README.md`) aggiornate nello stesso commit. Il report/chiusura di issue GitHub/GitLab dalla GUI resta un follow-up (serve forge client + token).

## 0.113.0

- Desktop (CF-105, M27 — config editor v2): il form **Add endpoint** copre ora 10 check comuni — oltre a http/certs/tcp/dns sono stati aggiunti **tls, redis, nats, smtp, grpc, postgres**. `engine.AddEndpoint` è stato esteso di conseguenza (target scalari per tls/redis/nats; mappa per smtp/grpc/postgres, con un campo `Extra` che diventa `service` per grpc e `password_env` per postgres), sempre preservando commenti e formattazione dello YAML. Aggiunta la **validazione live**: mentre scrivi nell'editor un badge mostra `✓ valid` o `✕ N problems` (dettagli in tooltip), la stessa `engine.Validate` del pulsante Validate ma sul testo non salvato. Nessun segreto nella UI — per postgres si indica il *nome* della variabile d'ambiente della password, non la password. TDD: `TestAddEndpointMoreKinds` nell'engine + firma del binding `App.AddEndpoint` aggiornata; docs (`docs/desktop.md`, `desktop/README.md`) aggiornate con screenshot.

## 0.112.2

- Docs (desktop, dettaglio): aggiunti gli **screenshot** delle sezioni nuove — `docs/assets/desktop-dashboard.png` (tutti i grafici) e `desktop-history.png` (drawer del history browser), generati dal frontend statico via Chrome headless. Arricchita `docs/desktop.md` con una **tabella delle scorciatoie da tastiera**, l'elenco dei comandi della **command palette** e una nota sugli stati (progress bar, spinner, toast, error card, `prefers-reduced-motion`, accessibilità).

## 0.112.1

- Docs (sync desktop): aggiornate `docs/desktop.md` (sito) e `desktop/README.md` alle feature spedite in M25/M26/M27, che erano rimaste indietro. Aggiunte le sezioni **Dashboard** (stacked-area, donut, banda worst-status, heatmap modulo×run, availability/SLO, metric-over-time), **History browser** (sfoglia/confronta run passati) e "**Views, command palette & shortcuts**". Corretti riferimenti stale: l'editor si apre dalla **tab Config** (non più dal bottone ⚙), e la tabella dei finding ora ha la colonna **Trend** con sparkline inline. Nessun cambio di codice.

## 0.112.0

- Modulo `cassandra`/`scylla` (CF-79, M21 — completamento): il modulo di v0.79.0 copriva la reachability ma **non lo "stato nodi"** chiesto dalla spec. Aggiunto un finding `cluster` che aggrega le probe: quanti nodi accettano CQL su quelli configurati, come **metrica** (unit `nodes`). **BAD** sotto `expect_nodes` (0 = li si vuole tutti), **WARN** quando la soglia è raggiunta ma un nodo configurato è comunque giù, **OK** se sono tutti su. Un nodo lento (WARN per `max_latency_ms`) conta come **up**: l'handshake l'ha completato. Il rollup è omesso con un solo target senza `expect_nodes`, dove ripeterebbe soltanto il finding del nodo.
- Limite dichiarato apertamente (docs + backlog): lo stato è derivato dalle **probe di checkfleet**, non da `system.peers` — leggere la vista che il cluster ha di sé richiede una `QUERY` su sessione autenticata, mentre questo modulo parla solo l'handshake. Il compromesso è voluto: non vede i nodi assenti dalla config, ma continua a funzionare sui cluster con auth attiva, senza driver né credenziali. Stessa forma di `expect_members` nel modulo `etcd`.
- Modulo `cassandra`: la latenza di handshake diventa una **metrica** (`Value`+`Unit` `ms`), come già fa `grpc` — allinea il modulo alla convenzione CF-91/CF-97.
- Config: nuovo `checks.cassandra.expect_nodes`, validato (non negativo e non superiore al numero di target configurati — un'aspettativa insoddisfacibile è un errore di config, non un BAD a runtime). Aggiornati `checkfleet.example.yml`, scaffold, `internal/moduledoc`, `docs/modules.md` e README. Test: 9 nuovi casi (rollup table-driven su 5 scenari, omissione per singolo nodo, metrica di latenza, end-to-end `Run` con un nodo su e uno giù).
- **Chiude M21 (Più datastore/infra)**: CF-76 clickhouse, CF-77 vault, CF-78 memcached, CF-79 cassandra.

## 0.111.0

- Modulo `memcached` (CF-78, M21 — completamento): il modulo consegnato in v0.78.0 copriva reachability, memoria e connessioni ma **non le evictions**, che la spec del backlog chiedeva. Aggiunto un finding `<target> [evictions]` con il contatore. Scelta di design: memcached espone solo il **totale dall'avvio**, non un rate, quindi non esiste una soglia di default sensata — il contatore è pubblicato come **metrica numerica** (`Finding.Value`, unit `evictions`) così lo storico lo grafica nella card "Metric over time" e la pendenza della linea diventa il segnale vero; il WARN scatta solo con `evictions_warn` esplicito (0 = solo report). Il finding è omesso se il server non riporta la stat.
- Modulo `memcached`: la percentuale di memoria diventa anch'essa una **metrica** (`Value`+`Unit` `%`), allineando il modulo alla convenzione di CF-91/CF-97 che gli era sfuggita. Messaggi e status invariati.
- Config: nuovo `checks.memcached.evictions_warn` (validato ≥ 0), presente in `checkfleet.example.yml`, nello scaffold (`internal/scaffold`) e in `internal/moduledoc`. Docs `docs/modules.md` aggiornate. Test: 4 nuovi casi (metrica memoria, evictions come metrica, WARN sopra soglia, stat assente → nessun finding) contro il finto memcached in-test.

## 0.110.0

- Desktop (CF-104, M27 — history browser & confronto run): nuovo bottone **History** che apre un drawer con i run persistiti (`internal/history`), newest-first, ciascuno con badge del worst status, timestamp e conteggi OK/WARN/BAD/ERROR. Aprendo un run se ne vedono i finding (status + valore numerico; i messaggi non sono salvati nello storico) con un'azione **Compare with previous** che mostra il delta rispetto al run precedente (new/resolved/worsened/improved), riusando `engine.DiffStatus`. Tre binding nuovi — `App.HistoryRuns`, `App.RunAt`, `App.DiffRuns` — coperti da `TestHistoryBrowser`. Va oltre il diff in-sessione (CF-64) e la sparkline di trend (CF-70). Apre M27 (Desktop power & workflow).

## 0.109.1

- CI (fix workflow "Backlog sync"): `backlog-sync` moriva con `gh api -X POST … milestones -f title=M26 — Desktop UX/UI polish: exit status 1: gh: Validation Failed (HTTP 422)`. Causa: `ensureMilestones` leggeva le milestone **senza paginazione** e l'API GitHub ne restituisce **30 per pagina** — con 31 milestone sul repo, M26 (l'ultima) cadeva fuori dalla prima pagina, il tool la credeva mancante e provava a ricrearla ottenendo `already_exists`. Aggiunti `--paginate` + `per_page=100`. La rottura era latente e avrebbe colpito ogni milestone dalla 31ª in poi: non è un problema di M26 in sé.
- `backlog-sync` (robustezza): un `already_exists` sulla creazione di una milestone non è più fatale — lo stato desiderato vale comunque (creazione concorrente o manuale), coerente con l'idempotenza dichiarata del tool. Per poterlo distinguere, gli errori di `gh` ora includono anche **stdout**: il body JSON dell'API finisce lì, mentre su stderr arriva solo `Validation Failed (HTTP 422)`, che da solo non dice quale validazione sia fallita. Test su `isAlreadyExists` con il body 422 reale.

## 0.109.0

- Desktop (CF-103, M26 — accessibilità & tastiera): drawer e command-palette ora hanno **focus-trap** (Tab resta dentro il dialog) e **ripristino del focus** al controllo che li ha aperti alla chiusura. ARIA: `role="dialog" aria-modal` su drawer/palette, la palette è un `combobox` su una `listbox` con `role="option"`/`aria-selected`/`aria-activedescendant`, la barra di progresso è `role="progressbar"`, il filtro ha `aria-label`. I chip dei moduli sono attivabili da tastiera (`role="button"` + Enter/Space). Alzato il contrasto del testo secondario in tema chiaro ad AA. Il focus-ring unico (`:focus-visible`) e `prefers-reduced-motion` erano già in CF-98. **Chiude M26 (polish UX/UI desktop).** Solo frontend statico.

## 0.108.0

- Desktop (CF-102, M26 — rifinitura Fleet & Dashboard): la tabella dei finding ha una nuova colonna **Trend** con una **sparkline inline per target** (linea minimale senza assi, `charts.js` → `svgSparkline`), disegnata per i target che hanno una serie numerica nello storico (`App.Metrics`, caricato dopo ogni Run). I grafici della dashboard sono più leggibili: i punti del line-chart ora hanno un **tooltip nativo `<title>`** con valore e unità. Header sticky, hover e pill di status erano già a posto. Smoke esteso con un caso `svgSparkline`; stylelint verde. Solo frontend statico.

## 0.107.0

- Desktop (CF-101, M26 — toast in-app): nuove notifiche non-bloccanti al posto (in aggiunta) del solo testo nella status bar. Helper `toast(msg, {kind, timeout, action})` + un container `aria-live="polite"`: card con dot per tipo (info/success/warn/error), auto-dismiss (in pausa se ci passi sopra col mouse), pulsante di chiusura e azione opzionale, stack limitato a 4. Agganciato alle azioni: export ("Exported … → path" / errore), salvataggio config, validate ("valid" / "N problems"), copy ("Copied to clipboard"). Enter/exit animati via classi, `prefers-reduced-motion` rispettato. Solo frontend statico.

## 0.106.0

- Desktop (CF-100, M26 — micro-interazioni & motion): la GUI ora ha transizioni curate, tutte in CSS (nessun refactor JS). Cambio vista con **fade-up**, drawer che **scivola** da destra, command-palette in **pop-in**, scrim in dissolvenza, grafici che compaiono in fade; feedback hover/press più vivo su tab, bottoni, righe della tabella e chip. Tutto rispetta `prefers-reduced-motion` (animazioni azzerate). Le transizioni sui selettori già esistenti sono fuse nelle regole base per non violare lo stylelint. Solo `dist/style.css`.

## 0.105.0

- Desktop (CF-99, M26 — stati loading/vuoto/errore): la GUI non mostra più schermate "morte" durante le attese. Aggiunti: **barra di progresso indeterminata** in cima + **spinner** sul bottone Run mentre gira; **skeleton shimmer** nei grafici della dashboard durante i fetch dello storico; empty-state curati (loading, "press Run" con scorciatoie da tastiera, no-match con "Clear filters"); una **error card inline** quando la config non gira, con messaggio e azioni **Retry** / **Open config editor**; stati `disabled` coerenti. Reso via helper `emptyState()`/`setBusy()`, `prefers-reduced-motion` rispettato. Solo frontend statico; stylelint e smoke verdi.

## 0.104.3

- CI (fix, due workflow rossi):
  - **`lint` — golangci-lint v2**. Sostituito il workaround di 0.104.2 (`run.go: "1.24"`, che abbassava il target di analisi per accontentare un binario vecchio) con la soluzione vera: **golangci-lint v2.12.2** + `golangci-lint-action@v9`. v1 è EOL e i suoi ultimi binari ufficiali sono buildati con go1.24, quindi rifiutano il target go1.25 di `go.mod`. `.golangci.yml` migrato allo **schema v2**: l'`issues.exclude-use-default` di v1 non esiste più, quindi le stesse esclusioni built-in sono ora esplicite in `linters.exclusions.presets` (senza, sarebbero comparsi 69 errcheck su `defer Close` mai considerati prima). Sistemati i 5 finding **nuovi** degli analizzatori più recenti: `reflect.Ptr` → `reflect.Pointer` (`internal/engine/validate.go`), 3 stringhe d'errore maiuscole in `cmd/checkfleet/main.go` (ST1005) e una dichiarazione ridondante in `ntp_test.go` (ST1023). Nessun cambio di comportamento; `golangci-lint run` verde in locale con v2.12.2.
  - **`frontend-lint` — xmllint mancante**. Lo step "SVG well-formed" falliva con `xmllint: command not found` (exit 127): il binario non è più preinstallato sulle immagini Ubuntu hosted di GitHub. Aggiunto uno step che installa `libxml2-utils` prima del controllo.
- Docs: `docs/development.md` aggiornato con la nuova versione pinnata e il vincolo "il binario di golangci-lint deve essere buildato con Go >= quello di `go.mod`".

## 0.104.2

- CI (fix gate golangci-lint): il job `lint` falliva da diverse commit con _"the Go language version (go1.24) used to build golangci-lint is lower than the targeted Go version (1.25.0)"_ — `go.mod` targetta go1.25 ma la `golangci-lint-action` scarica il binario ufficiale v1.64.8 **buildato con go1.24**, che rifiuta un target più recente. Aggiunto `run.go: "1.24"` in `.golangci.yml` così l'analisi targetta 1.24 (i linter abilitati — errcheck/govet/ineffassign/staticcheck/unused — non dipendono dalla semantica 1.25). Nessun finding reale nel codice (verificato in locale). Da rivedere quando si passa a golangci-lint v2.

## 0.104.1

- Desktop (fix CI): il design-system pass di CF-98 ri-dichiarava `.worst b`, `.tile b` e `.btn-primary` in coda, facendo fallire lo stylelint del workflow "Desktop tests" (`no-duplicate-selectors`). Fuse quelle proprietà nelle regole originali; stylelint di nuovo verde (verificato in locale con `stylelint-config-recommended`). Nessun cambiamento visivo.

## 0.104.0

- Desktop (CF-98, M26 — design system pass): introdotto un layer di **design token** nel CSS della GUI (zero-dep, nessun build) — scala di spaziatura, raggi, scala tipografica, elevazioni ombra (`--shadow-sm/md`), motion (`--ease`/`--dur`) e un **focus-ring** unico. Bottoni/input/badge/chip/card ora condividono raggi e stati coerenti: `:focus-visible` uniforme su tutti i controlli, feedback "press" sui bottoni, elevazione + lift in hover sulle card, gerarchia più forte nel summary (worst-status come numero hero, cifre tabulari) e rispetto di `prefers-reduced-motion`. Parità light/dark verificata. Solo `desktop/frontend/dist/style.css`; nessun cambio a JS/binding/engine. Apre M26 (polish UX/UI desktop).

## 0.103.0

- Engine/moduli (CF-97, M25): esteso il campo numerico opzionale `Finding.Value`+`Unit` (CF-91) ad altri **7 moduli**, così le loro grandezze finiscono nello storico e sono graficabili nella card "Metric over time" della GUI: **latenza ms** (`grpc`, aggiunto il timing della richiesta), **giorni-a-scadenza** (`tls`), **replica lag** (`mysql` in s, `clickhouse` in s, `postgres` in bytes, `patroni` in bytes) e **min-TTL** (`dns`, in s). Backward-compatible: i renderer `internal/output` e i messaggi restano invariati. Asserzioni `Value` aggiunte a `tls`/`grpc`; gli altri test esistenti restano verdi (nessuna regressione). Completa il follow-up lasciato aperto da CF-91.

## 0.102.4

- Desktop (fix icona — via il bianco intorno): `appicon.png` aveva gli **angoli bianchi** invece che trasparenti, quindi nel Dock/Finder l'icona mostrava un brutto quadrato bianco attorno al plate arrotondato. Causa: `qlmanage` cuoce uno sfondo bianco quando rende l'SVG. `gen-icon.sh` ora rende con **Chrome headless** (`--default-background-color=00000000`), che preserva il canale alpha → angoli trasparenti; `appicon.png` rigenerato di conseguenza. L'SVG era già corretto (angoli trasparenti), il baco era solo nel rasterizzatore. Verificato: pixel d'angolo `(0,0,0,0)`.

## 0.102.3

- Desktop (logo ridisegnato): il check e l'arco "c" si sovrapponevano in modo confuso (e il tentativo di separarli con una fascia scura si leggeva come una linea nera/glitch a dimensione dock). Ridisegnata la geometria: il check verde ora è **annidato dentro la "c"** e il suo braccio lungo esce pulito dall'apertura a destra **senza incrociare il teal** — i due segni restano distinti, niente sovrapposizioni né knockout. `appicon.png` + iconset completo rigenerati. Solo asset frontend.

## 0.102.2

- Desktop (icona completa): nuovo script `desktop/build/gen-icon.sh` che rigenera `appicon.png` dall'SVG e costruisce un **iconset `.icns` completo** — tutte le taglie da 16 a 1024 in 1x **e** 2x (Wails da solo emette solo le varianti @2x). Agganciato al workflow `desktop.yml` (step macOS dopo `wails build`), così anche gli artefatti di release hanno l'icona a piena risoluzione a ogni dimensione. Zero-dep: usa solo `qlmanage`/`sips`/`iconutil` di sistema.

## 0.102.1

- Desktop (fix logo): rimossa la fascia scura di separazione che avevo aggiunto attorno al check — a dimensione dock si leggeva come una linea nera/glitch. Ora il check verde è disegnato pulito sopra l'arco teal (bordi netti, niente glow, niente knockout). `appicon.png` rigenerato dall'SVG. Solo asset frontend.

## 0.102.0

- Desktop (CF-96, M25): **shell UX v3**. Le tre schermate (Fleet / Dashboard / Config) diventano **viste di prima classe** con tab nel titlebar, guidate da un unico `setView` (niente più toggle separati). Nuova **command-palette** (⌘K): campo di ricerca, navigazione con frecce + Enter, click, che invoca le stesse azioni del resto della UI (run, cambio vista, validate, trend, export, tema). **Keyboard shortcut**: ⌘K palette, ⌘↵ run, `1/2/3` per le viste, `/` per il filtro, `r` per run, Esc per chiudere. Layout responsive (la griglia della dashboard collassa su schermi stretti) e la vista corrente è persistita in localStorage. **Chiude M25 (Desktop GUI v3 — dashboard & grafici).** _(solo frontend statico)_

## 0.101.0

- Desktop (CF-94, M25): grafici di **metrica nel tempo** nella Dashboard. Nuovo binding `App.Metrics` che, dallo storico persistente, estrae una serie temporale per ogni check/target che porta un `Value` numerico (CF-91). Il frontend mostra una card **Metric over time** con selettore della serie e un **line chart** SVG (`charts.js` → `svgLine`, zero-dep, con gridline/assi e scala y a ridosso dei dati); in più il **drawer di un finding** con metrica ne disegna la serie storica inline. Binding coperto da `TestMetrics`, geometria da smoke `svgLine`. Nessun cambio all'engine (usa il campo già introdotto in CF-91). _(solo frontend statico + binding desktop)_

## 0.100.0

- Engine (CF-91, M25): `engine.Finding` guadagna due campi opzionali **`Value *float64` + `Unit string`** (JSON `omitempty`, helper `engine.Num`) per allegare la grandezza misurata da un check — **backward-compatible**: i renderer `internal/output` restano invariati e i finding senza metrica non cambiano output. `internal/history` persiste `v`/`u`, così la GUI può graficare la metrica nel tempo (base per CF-94). Popolato nei moduli con uno scalare chiaro per finding: latenza in ms (`http`, `tcp`), offset dell'orologio in ms (`ntp`), giorni-a-scadenza (`certs`). Test: engine (omitempty), http (Value sull'OK, assente sull'ERROR), certs (unit `days`). I restanti moduli (dns/tls/grpc + lag di patroni/postgres/mysql/clickhouse) si aggiungono in modo incrementale.

## 0.99.0

- Desktop (CF-95, M25): card **Availability / SLO** nella Dashboard. Nuovo binding `App.Availability` che dallo storico persistente (`internal/history`) calcola l'**uptime** della flotta (quota di run col worst status OK) sulla finestra recente, la **streak** dello stato corrente (da quando dura) e l'**uptime per-target** ordinato dal meno disponibile. Il frontend mostra un hero con la percentuale + stato corrente e la lista dei target peggiori con una barra SVG (`charts.js` → `svgMeter`, zero-dep, colorata per soglia SLO). Binding e helper `pct` coperti da test (`TestAvailability`, `TestPct`, smoke `charts.test.js`). Nessun cambio all'engine. _(solo frontend statico + binding desktop)_

## 0.98.3

- Desktop (fix logo): il mark aveva un glow gaussiano sul check che lo faceva leggere come una sagoma sfocata/non ritagliata; sostituito con un mark netto + una fascia scura di separazione (`#0b1327`) così il check risulta pulitamente scontornato sopra l'arco teal. Inoltre **rigenerato `desktop/build/appicon.png`** dall'SVG rifinito: era fermo allo scaffold iniziale (v0.26.0), quindi il dock mostrava ancora il logo pre-rifinitura. Solo asset frontend (`desktop/frontend/dist/assets/logo.svg` + sync `docs/assets/logo.svg` + `appicon.png`); l'icona dell'app si rigenera al `wails build`.

## 0.98.2

- Desktop: **primo avvio senza config** ora crea uno starter valido invece di aprire una schermata vuota (che sembrava rotta). Se non c'è né `CHECKFLEET_CONFIG` né `./checkfleet.yml`, `StartupConfig` genera un `checkfleet.yml` starter (via `internal/scaffold`, moduli certs+http) nella cartella config utente (`os.UserConfigDir()/checkfleet/`) e apre quello; `Startup` ora riporta `created`/`note` così la GUI può segnalarlo. Non sovrascrive mai un file esistente. `ensureStarterConfig` testato (crea + valida + idempotente).

## 0.98.1

- Desktop DX: `wails dev` non partiva (`frontend:dev:serverUrl: "auto"` senza watcher, con frontend statico → errore "unable to auto discover"). Impostato `frontend:dev:serverUrl: ""` così Wails serve direttamente `frontend/dist/` con hot-reload; ora `wails dev` funziona (finestra nativa + DevTools su http://localhost:34115 con i binding Go reali → dati veri, non il mock). README desktop aggiornato.

## 0.98.0

- Timeout/retry **per-modulo** (CF-84, M23): nuova sezione `module_overrides` nel config — per ogni modulo si possono sovrascrivere `timeout_seconds`/`retries`/`retry_backoff_ms` (un campo a zero eredita il globale). Utile quando un modulo è più lento o più flaky degli altri (es. `postgres: {timeout_seconds: 10, retries: 2}`) senza cambiare i default globali. Nuovo `engine.Job` (check + Options) e `engine.RunJobs` (esegue ogni job con le sue Options); `RunWith` ora è un wrapper a Options uniforme sopra `RunJobs`, quindi il desktop resta invariato. `registry.OptionsFor`/`Jobs` applicano il merge; usati da `check`/`serve`/`--watch`. Testati (merge override + ordine dei job). **Chiude M23 (Engine & robustezza) e le milestone M21–M24.**

## 0.97.1

- CI/frontend (CF-97): il job `frontend-smoke` ora, oltre ad asserire che la fleet view si renderizzi, **cattura uno screenshot** della UI (render headless con dati mock) e lo carica come artifact **`frontend-preview`** — così ogni run porta con sé un'anteprima visiva scaricabile del frontend, senza dover buildare/lanciare l'app. Reso a 1280×860 @2x (`--force-device-scale-factor=2`), con una nota nel job summary che punta all'artifact. Riusa il Chrome già installato nel job (nessun setup extra).

## 0.97.0

- Maintenance window **ricorrenti** (CF-85, M23): oltre alle finestre una-tantum (`from`/`to`), una finestra può ora ripetersi ogni giorno con `daily: "HH:MM-HH:MM"` (orario locale, gestisce il wrap oltre la mezzanotte), opzionalmente ristretta a certi `weekdays` (es. `[Sat, Sun]`). `from`/`to` continuano a delimitare la validità complessiva (es. finestra notturna valida solo per un mese). Un range `daily` malformato non silenzia nulla (fail-safe). Nuovi campi `daily`/`weekdays` su `MaintenanceWindow`; logica testata (dentro/fuori finestra, wrap notturno, restrizione weekday). Documentato in `configuration.md`.

## 0.96.1

- CI/frontend (CF-97): nuovo job **`frontend-lint`** in `desktop-test.yml` che valida gli asset statici del desktop — finora coperti solo dallo smoke-render, che non intercetta uno stylesheet rotto o un SVG malformato. `stylelint` (config recommended, `no-descending-specificity` disattivata perché rumorosa) sul CSS, `html-validate` (recommended; disattivate `doctype-style` e `no-implicit-button-type` opinabili, e `no-inline-style` perché Wails **richiede** l'attributo `--wails-draggable` inline sulle drag-region della titlebar) sull'HTML, `node --check` sulla sintassi di `main.js` e `xmllint` sugli SVG. Le config vivono in `desktop/frontend/{.stylelintrc.json,.htmlvalidate.json}`; i linter si installano freschi in CI (`npm install --no-save`, `node_modules/` ora in `.gitignore`). Il linter ha già scovato e fatto correggere 2 selettori CSS duplicati (`.findings tbody tr:hover` scritto due volte, `.chip` definito in due punti).

## 0.96.0

- Dedup & ordinamento documentato (CF-86, M23): il runner ora **deduplica** i finding esattamente identici (stesso check/target/status/message) mantenendo il primo e preservando l'ordine — utile quando lo stesso target è listato due volte o un modulo emette lo stesso finding due volte (`engine.Dedup`). L'**ordinamento** dei finding è formalizzato come API di fatto valida per **tutti** gli output: worst-first, poi check, poi target, con sort **stabile**; documentato in `docs/output.md`. Testato (`Dedup` + un run che deduplica). Apre M23 (Engine & robustezza).

## 0.95.0

- Logging strutturato opzionale su `serve` (CF-89, M24): nuovo flag `--log-format text|json` (default `text`) via `log/slog` — logga `serve start` (moduli, listen, interval) e ogni `run complete` (durata_ms, worst, conteggi ok/warn/bad/error) in testo leggibile o **JSON** per le pipeline di log. **Chiude M24 (Osservabilità del tool).**
- Fix: `output.SelfMetrics` (CF-87) duplicava `checkfleet_run_duration_seconds` e `checkfleet_last_run_timestamp_seconds`, già emesse da `output.Prometheus` → **metric family duplicata** sullo scrape `/metrics`. Ora `SelfMetrics` emette solo le metriche per-modulo (`checkfleet_module_findings`/`checkfleet_module_errors`); un test lo verifica.
- Fix: `checkfleet init` non copriva i moduli aggiunti in M20/M21 (mysql, etcd, clickhouse, vault, memcached, cassandra) — la guard `TestSupportedCoversRegistry` era rossa. Aggiunti gli snippet mancanti in `internal/scaffold` (ognuno carica e valida); ora `init --list` e `init --modules` li includono e la guard è verde.

## 0.94.0

- Desktop (CF-93, M25): **heatmap per modulo** nella Dashboard. Nuovo binding `App.TrendByModule` che collassa lo storico persistente (`internal/history`) nel worst status di ogni modulo per run; il frontend lo disegna come griglia modulo×run color-coded (`charts.js` → `svgHeatmap`, zero-dep, theme-aware): una riga per modulo, una colonna per run, cella colorata per status (assenza del modulo = cella vuota), con scroll orizzontale quando i run sono tanti. Click su una riga/cella → drawer con la **banda worst-status** di quel modulo nel tempo. Binding e geometria coperti da test (`TestTrendByModule`, `TestWorseOf`, smoke `charts.test.js`). Nessun cambio all'engine. _(solo frontend statico + binding desktop)_

## 0.93.0

- Endpoint `/healthz` & `/readyz` su `serve` (CF-88, M24): **liveness** (`/healthz` → `ok`, il processo è su) e **readiness** (`/readyz` → `503` finché il primo run non è completato, poi `200`) per le probe di Kubernetes/Nomad. La readiness diventa vera quando il primo giro di check popola `/metrics`, così l'orchestratore non manda traffico/allarmi prima che ci siano dati. La root `/` elenca gli endpoint disponibili.

## 0.92.0

- Self-metrics dell'exporter (CF-87, M24): l'endpoint `/metrics` di `serve` ora espone anche metriche **sul tool stesso**, non solo sui target — `checkfleet_run_duration_seconds` (durata dell'ultimo run), `checkfleet_last_run_timestamp_seconds` (quando è partito), e per modulo `checkfleet_module_findings{module}` e `checkfleet_module_errors{module}` (ERROR = misurazioni fallite). Così si può allertare sul checker stesso (run bloccati, modulo che va sempre in errore). Renderer `output.SelfMetrics` testato. Apre M24 (Osservabilità del tool).

## 0.91.0

- Alert **AWS SNS** (CF-83, M22): `checkfleet alert --provider sns --sns-topic-arn <arn>` pubblica ogni finding BAD/ERROR su un topic SNS — sink stateless, quindi solo publish (nessun resolve). Le richieste sono firmate con **AWS Signature V4 scritta a mano** in un nuovo package condiviso `internal/awssig` (zero-dep, nessun SDK AWS, gestisce il body per le POST); la region è ricavata dal topic ARN e le credenziali arrivano **dall'ambiente** (`--aws-access-key-env`/`--aws-secret-key-env`, default `AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY`). Payload SNS `Publish` form-encoded (`internal/alert.SNSForm`, subject sanitizzato ≤100 char). Firma e form testati (deterministici, con body diversi → firme diverse). **Chiude M22 (Alerting & sink).**

## 0.90.0

- Desktop (CF-92, M25): nuova vista **Dashboard** con grafici SVG disegnati a mano — **zero-dep, theme-aware** (ogni elemento è colorato via classe CSS, così il tema chiaro/scuro funziona da solo). Toggle nel titlebar, mutuamente esclusivo con l'editor di config. Tre grafici alimentati dallo storico persistente (binding `Trend` su `internal/history`) più il run corrente: **stacked-area** dei conteggi OK/WARN/BAD/ERROR per run (con gridline e assi), **donut** della distribuzione corrente (100% reso come anello, caso vuoto gestito) e **banda worst-status** nel tempo con asse temporale; refresh automatico dopo ogni Run. Il modulo grafici `desktop/frontend/dist/charts.js` espone funzioni pure coperte da uno smoke headless `desktop/frontend/charts.test.js` (`node --test`, 9 casi). Nessun binding Go nuovo, nessun cambio all'engine — solo frontend statico. **Apre M25 (Desktop GUI v3 — dashboard & grafici).** _(la numerazione salta a 0.90.0 per tenere la corsia GUI separata dalle release dei moduli)_

## 0.82.0

- Webhook con **template custom** (CF-82, M22): `--output webhook --template FILE` plasma il payload con un **Go text/template** invece del JSON di default. Il template viene eseguito su `{{.Title}}`, `{{.Worst}}`, `{{.Total}}`, `{{.OK}}`/`{{.WARN}}`/`{{.BAD}}`/`{{.ERROR}}` e `{{range .Findings}}` (con `.Check`/`.Target`/`.Status`/`.Message`). `missingkey=error` fa emergere i typo nel template invece di emettere silenziosamente `<no value>`. Renderer `output.RenderTemplate` testato. Senza `--template` il comportamento resta il JSON di prima.

## 0.81.0

- Output `csv` (CF-81, M22): `--output csv` emette i finding come CSV con header (`status,check,target,message`), worst-first, un finding per riga. I campi sono quotati/escapati via `encoding/csv` (virgole e newline nei messaggi sono al sicuro) — comodo per fogli di calcolo o ingestione. Renderer `output.CSV` testato (incluso l'escaping). CLI `--output csv` (spesso con `--out-file`).

## 0.80.0

- Output `telegram` (CF-80, M22): `--output telegram` invia il report come messaggio di testo via **Telegram Bot API** (`sendMessage`) — riga di summary + finding non-OK (worst-first, capped entro i 4096 caratteri). Token del bot e chat id **da env** (`--telegram-token-env` default `TELEGRAM_TOKEN`, `--telegram-chat-env` default `TELEGRAM_CHAT_ID`), mai da CLI. Testo semplice (niente escaping MarkdownV2). Renderer `output.Telegram` testato. Apre M22 (Alerting & sink).

## 0.79.0

- Modulo `cassandra`/`scylla` (CF-79, M21): reachability parlando direttamente il **protocollo CQL nativo** (handshake `OPTIONS`→`SUPPORTED`, `STARTUP`) — **zero-dep, nessun driver, nessuna auth**. Una risposta `READY` o `AUTHENTICATE` significa che il nodo accetta connessioni CQL → OK (con nota `(auth required)` quando l'auth è attiva); **WARN** se l'handshake supera `max_latency_ms`, **BAD** su risposta `ERROR` di protocollo, **ERROR** se il nodo è irraggiungibile. Frame CQL v4 scritti a mano (header + string map/multimap). Target `host[:port]` (default 9042). Testato contro un finto server CQL in-test. CLI `checkfleet check cassandra`. **Chiude M21 (Più datastore/infra).**

## 0.78.0

- Modulo `memcached` (CF-78, M21): health check di memcached via il **protocollo testuale** (zero-dep). Apre la connessione, esegue `STATS` (ERROR se irraggiungibile), e segnala **WARN** quando `bytes` supera `mem_warn_pct` di `limit_maxbytes` (default 90); altrimenti OK con versione, percentuale di memoria e numero di connessioni. Target `host[:port]` (porta di default 11211). Testato contro un finto memcached in-test. CLI `checkfleet check memcached`.

## 0.77.0

- Modulo `vault` (CF-77, M21): health check di HashiCorp Vault via HTTP (zero-dep). `/v1/sys/seal-status` → **ERROR** se irraggiungibile, **BAD** se il nodo è **sealed** (con progresso di unseal `n/soglia`) o non inizializzato; `/v1/sys/health` → ruolo **active/standby** (entrambi OK — lo standby è normale in HA) con la versione di Vault. Entrambi gli endpoint sono non autenticati; `token_env` opzionale invia `X-Vault-Token`, `insecure_skip_verify` per HTTPS self-signed. Il corpo JSON viene letto anche sugli status non-200 (Vault codifica lo stato nel body su 429/503). Testato contro un finto Vault (`httptest`). CLI `checkfleet check vault`.

## 0.76.1

- Desktop (CF-90, M18): **rifinitura grafica del logo e della UI**. Il mark dell'app (plate slate scuro + arco "c" teal che culla il check smeraldo) prima si confondeva con la titlebar quasi dello stesso colore: ora ha un gradiente slate più chiaro, un glow smeraldo ambientale, un rim-light in alto e un anello hairline, così si stacca e legge come una vera app-icon. Più respiro attorno al brand (gap 8→11px, mark 22→24px con ring + ombra morbida), la pill di versione (`dev`/tag) diventa un chip brand-tinted, l'empty-state passa da logo sbiadito al 50% a mark a piena forza con alone brand, e micro-transizioni su bottoni/input/mark. Solo frontend statico (`desktop/frontend/dist/{index.html,style.css,assets/logo.svg}` + sync `docs/assets/logo.svg`), nessun binding Go nuovo.

## 0.76.0

- Modulo `clickhouse` (CF-76, M21): health check di ClickHouse via l'interfaccia **HTTP** (zero-dep). `/ping` (ERROR se irraggiungibile, BAD se non risponde `Ok.`) + `SELECT version()` per la reachability e la versione; per ogni **tabella replicata** (`system.replicas`) segnala **BAD** se la replica è read-only (tipicamente sessione ZooKeeper/Keeper persa) e il **ritardo di replica** (`absolute_delay`) con WARN/BAD oltre `delay_warn_seconds`/`delay_crit_seconds` (default 30/300). Le tabelle sane non producono finding. Credenziali **da env** (basic auth `username`+`password_env`), `insecure_skip_verify` per HTTPS self-signed. Testato contro un finto server HTTP (`httptest`). CLI `checkfleet check clickhouse`. Apre M21 (Più datastore/infra).

## 0.75.0

- Modulo `etcd` (CF-75, M20): health check di un cluster etcd v3 via il suo **HTTP JSON gateway** (nessuna dipendenza `clientv3`). `/health` (ERROR se irraggiungibile, BAD se unhealthy), `POST /v3/maintenance/status` → **BAD se non c'è un leader** (quorum perso) + versione etcd, `POST /v3/cluster/member/list` → **BAD** se i membri sono meno di `expect_members` (rischio quorum). Token auth opzionale (`username`+`password_env` → `/v3/auth/authenticate`) e `insecure_skip_verify` per cluster self-signed. **Zero dipendenze** (HTTP/JSON); i 64-bit (leader/member id) arrivano come stringhe dal gateway proto3. Testato contro un gateway finto (`httptest`). CLI `checkfleet check etcd`. **Chiude M20 (Più datastore).**

## 0.74.0

- Modulo `mysql`/`mariadb` (CF-74, M20): health check read-only. Server raggiungibile (con versione e ruolo **read-only**), **saturazione connessioni** (`Threads_connected` vs `max_connections`, WARN oltre `conn_warn_pct` default 80), e su una **replica** lo stato dei thread IO/SQL (BAD se fermi) e il **lag** (`Seconds_Behind`, WARN/BAD oltre `lag_warn_seconds`/`lag_crit_seconds` default 10/60; BAD se non sta replicando). Gestisce sia `SHOW REPLICA STATUS` (MySQL 8.0.22+) sia il legacy `SHOW SLAVE STATUS` (MariaDB/vecchie versioni). Usa il driver standard `go-sql-driver/mysql` (eccezione motivata come `pgx`); la **password sta nell'ambiente** via interpolazione `${...}` nel DSN, mai inline. Logica dietro l'interfaccia `collector`, **testata con un fake** (nessun DB reale). CLI `checkfleet check mysql`. Apre M20 (Più datastore).

## 0.73.0

- CI/test (CF-73, M19): la CI ora esegue i test **con coverage** (`go test -coverprofile`) e stampa il totale nel job summary — visibilità, nessuna soglia hard che rompe la build. Colmati i buchi principali del core `engine`: `Validate` da 44% a 88% di copertura (rami di validazione per-modulo — stream/haproxy/patroni/consul/dns/smtp/elasticsearch/mongodb — più ordine soglie e range postgres), `ParseStatus` completo, e verificato che un modulo senza regole (es. `tcp`) conti come configurato; copertura di `internal/engine` da 74.9% a 83.9%. `cover.out` aggiunto a `.gitignore`. **Chiude M19 (Qualità).**

## 0.72.0

- CI/qualità (CF-72, M19): **`golangci-lint` promosso da advisory a gate**. Sistemati gli 11 finding sul tree (9 `errcheck` — return di `SetDeadline`/`Write`/`WriteString`/`ReadFull` nei moduli/test ora gestiti; 1 `unused` — campo morto in `internal/history`; 1 `staticcheck` SA4004 — loop che terminava sempre in `grpccheck`, riscritto senza loop). Aggiunto `.golangci.yml` con i linter fissati (errcheck, govet, ineffassign, staticcheck, unused) e rimosso `continue-on-error` in `ci.yml` con versione del linter pinnata. Tree pulito su modulo root **e** desktop. Completa CF-57 (apre M19 — Qualità).

## 0.71.0

- Desktop (CF-71, M18): **raggruppa per modulo**. Il nuovo toggle **Group** ripiega la tabella dei finding in sezioni collassabili, una per modulo, ognuna con un badge di rollup (lo status peggiore del modulo) e il conteggio dei finding; click sull'header per collassare/espandere. La scelta è ricordata tra i riavvii (localStorage). Utile per scorrere flotte con molti target modulo per modulo. **Chiude M18 (Prodotto & UX).**

## 0.70.0

- Desktop (CF-70, M18): **storico persistente & trend**. Ogni run viene appeso a un file JSONL nascosto accanto al config (`.<nome>.history.jsonl`, riusa `internal/history`); il nuovo bottone **Trend** apre un drawer con una **sparkline del worst-status per run** (barre verde/giallo/rosso/viola, dalla più vecchia alla più recente, con timestamp e conteggi OK/WARN/BAD/ERROR in tooltip). A differenza di **Changes** (solo in-sessione, CF-64), il trend **sopravvive ai riavvii**. Nuovo binding `App.Trend`; persistenza in `RunChecks`; testato (persistenza su file + lettura da una nuova istanza dell'app).

## 0.69.0

- Comando `checkfleet init` (CF-69, M18): scaffolding di un `checkfleet.yml` commentato e pronto da editare. `--modules certs,http,...` sceglie i moduli (default `certs,http`), `--list` elenca quelli disponibili, `--config` la destinazione; **non sovrascrive** un file esistente senza `--force`. Gli snippet per-modulo vivono in `internal/scaffold` (uno per ciascun modulo, con target/soglie placeholder e i segreti da env) e un test garantisce che ogni snippet generato **carichi e validi** senza problemi. Fix collegato: `engine.Validate` ora riconosce come "modulo configurato" anche i moduli senza regole esplicite (es. `tcp`, `tls`, `redis`) via reflection sui campi di `checks`, così un config con solo quei moduli non è più segnalato erroneamente come vuoto. Apre l'onboarding di M18.

## 0.68.0

- Output `text` a colori sul terminale (CF-68, M18): lo status è colorato con ANSI (OK verde, WARN giallo, BAD rosso, ERROR magenta) tramite `output.TextColor`. Il colore si attiva **solo quando l'output va a un terminale** (rilevamento TTY zero-dep via `ModeCharDevice`): resta disattivato su pipe, redirezione, `--out-file`, o con `NO_COLOR` (standard) e il nuovo flag `--no-color`. I codici ANSI sono a larghezza zero, quindi l'allineamento delle colonne è invariato. Vale anche per `--watch`. Apre **M18 (Prodotto & UX)**.

## 0.67.1

- Docs: tabella moduli del README allineata al codice — aggiunti `ingest`, `s3`, `smtp`, `elasticsearch`, `mongodb` (mancavano 5 dei 23 moduli). Aggiornata la riga roadmap: l'unico modulo ancora da fare è `mediamtx`.

## 0.67.0

- Modulo `mongodb` (CF-44): health check read-only di MongoDB. `replSetGetStatus` → **BAD** se manca un `PRIMARY` sano, **BAD** per membri con health 0 (irraggiungibili), e lag delle SECONDARY (**WARN** oltre `lag_warn_seconds`, **BAD** oltre `lag_crit_seconds`, default 10/60); un nodo standalone (non replica set) è riportato raggiungibile e i check di replica sono saltati. `serverStatus` → **WARN** su saturazione connessioni (`conn_warn_pct`, default 80). Credenziali **da env** (`username`+`password_env`, mai nell'URI/config), `auth_source` default `admin`. Usa il **driver ufficiale** `go.mongodb.org/mongo-driver/v2` — eccezione motivata alla regola zero-dep (come `pgx` per Postgres, per wire protocol BSON + auth SCRAM); la logica dei finding è dietro l'interfaccia `collector` e **testata con un fake** (nessun DB reale nei test). CLI `checkfleet check mongodb`.

## 0.66.0

- Modulo `elasticsearch`/`opensearch` (CF-25): salute cluster via HTTP API. `_cluster/health` → **green/OK, yellow/WARN, red/BAD** (con conteggio nodi, shard non assegnati e % shard attivi nel messaggio); `expect_nodes` → **BAD** se il cluster riporta meno nodi del previsto (cluster ridotto anche se green); **disk watermark per-nodo** via `_cat/allocation` → WARN oltre `disk_warn_pct` (default 85, low watermark ES), BAD oltre `disk_crit_pct` (default 90, high watermark). Credenziali **da env** (basic `username`+`password_env` o `api_key_env`), `insecure_skip_verify` per cluster self-signed. **Zero dipendenze** (HTTP/JSON); testato contro un cluster finto (`httptest`). CLI `checkfleet check elasticsearch`.

## 0.65.0

- Modulo `smtp` (CF-24): reachability di un relay SMTP — **non invia mai posta**. Verifica connessione + greeting `220` (opz. `expect_banner`), `EHLO`, e — se richiesto — **STARTTLS** (`starttls: true`, BAD se non offerto/fallisce) o **TLS implicito** (`tls: true`, es. porta 465); quando c'è TLS legge il certificato del relay e riporta la scadenza (`warn_days`/`crit_days`, come `certs`). WARN su `max_latency_ms`. Porta di default 25 (465 con `tls`). **Zero dipendenze** (stdlib `net`/`crypto/tls`/`bufio`, parsing risposte multi-linea a mano); testato contro relay fake in-test (plain, STARTTLS, TLS implicito, cert in scadenza/scaduto). CLI `checkfleet check smtp`.

## 0.64.0

- Desktop (CF-66, M17): **form "Add endpoint" + scheduling** nell'editor. **+ Add endpoint** apre un form rapido per i check più comuni — **http** (URL + status atteso), **certs** (`host:443`), **tcp** (`host:port`), **dns** (nome + tipo record): l'endpoint viene **inserito nello YAML** riusando `engine.AddEndpoint`, che edita il node tree yaml (commenti, ordine chiavi e formattazione **preservati**) e non tocca il resto. Bottone **Schedule…** che stampa comandi copia-incolla — riga `cron` + `checkfleet serve` per il file/intervallo correnti — così GUI e automazione condividono un'unica fonte. Nuovi binding `App.AddEndpoint`/`ScheduleSnippet` (helper `intervalMinutes`); testati (engine + desktop). Fix CSS: regola globale `[hidden]{display:none}` (il form/i campi condizionali usano `display:flex` che ignorava l'attributo `hidden`). **Chiude M17 (Config editor).**

## 0.63.0

- Desktop (CF-65, M17): **editor di configurazione** nella GUI. Il nuovo bottone ⚙ nella titlebar apre un editor YAML a tutto pannello con **Reload**, **Validate** e **Save**: si legge/scrive il file `checkfleet.yml` selezionato senza uscire dall'app. **Validate** controlla il testo *non salvato* (parse + regole di dominio) e mostra i problemi inline, riusando la validazione del CLI via il nuovo `engine.LoadBytes` (interpola `${...}`, applica i default, senza toccare il disco). Nuovi binding `ReadConfig`/`SaveConfig`/`ValidateText`; testati (engine + desktop). Apre **M17 (Config editor)**.

## 0.62.1

- CI/release (CF-67): **serializzate le release** per eliminare una race che lasciava tag senza release. `release.yml` (goreleaser) pubblica i tag **mutabili** `ghcr.io/allan-nava/checkfleet:latest-amd64`/`-arm64` a ogni tag `v*`; con più tag pushati a raffica i run concorrenti si sovrascrivevano a vicenda quei tag e la creazione del manifest `:latest` non riusciva a verificare il digest (`manifest verification failed for digest …`) → goreleaser ritentava per ~25 min e falliva, così la release non veniva creata e il job desktop (che aspetta la release per allegarci gli asset) andava in timeout con `release not found`. Aggiunto un `concurrency` group (`group: release`, `cancel-in-progress: false`) che accoda i run invece di lanciarli in parallelo; `desktop.yml` a sua volta serializzato e con attesa della release più generosa (fino a ~30 min) dato che ora le release possono essere in coda.

## 0.62.0

- Modulo `s3`/object storage (CF-23): verifica che un bucket S3-compatibile (AWS S3, MinIO, Ceph) sia raggiungibile e, opzionalmente, che un **oggetto sentinella** esista e sia fresco (`max_age_warn_seconds` → WARN se stantìo, BAD se mancante). Firma **AWS Signature V4 scritta a mano** (zero dipendenze, nessun SDK AWS); credenziali da env (`access_key_env`/`secret_key_env`) o richieste anonime per bucket pubblici; `path_style` per MinIO/Ceph. Testato contro un finto S3 (`httptest`). CLI `checkfleet check s3`. (Quota/spazio non esposti da S3 standard: rimandati.)

## 0.61.0

- Modulo `ingest` (CF-22): reachability degli endpoint di ingest streaming — "lo streamer riesce a pubblicare?". `protocol: rtmp` fa l'handshake RTMP semplice su TCP (C0/C1→S0/S1/S2, versione verificata); `protocol: srt` fa l'induction handshake SRT su UDP (reachability best-effort). OK/WARN(`max_latency_ms`)/ERROR(handshake fallito)/BAD(protocollo ignoto). **Zero dipendenze** (protocolli scritti a mano); testato contro finti server RTMP/SRT in-test. CLI `checkfleet check ingest`.

## 0.60.0

- Desktop (CF-64, M16): **vista storico/diff**. Il bottone **Changes (N)** apre un drawer col delta rispetto al run precedente della sessione — finding new/resolved/worsened/improved, color-coded — riusando `engine.DiffStatus`. `RunChecks` popola `Report.Changes` confrontando col run precedente in memoria; wiring testato. **Chiude M16 (Desktop GUI v2).**

## 0.59.0

- Desktop (CF-63, M16): **notifiche native & settings persistiti**. Nuovo toggle **Notify**: dopo un run con worst BAD/ERROR l'app fa una notifica desktop nativa (via `beeep`, binding `App.Notify`). Config path, stack, intervallo, Auto e Notify sono ora **ricordati tra i riavvii** (localStorage). Nuova dip del modulo desktop: `github.com/gen2brain/beeep` (solo GUI, il modulo CLI resta invariato).

## 0.58.0

- Desktop (CF-62, M16): **dettaglio & insight**. Click su un finding → drawer con messaggio completo e bottone **Copy**; click su una chip modulo → **Explain** (cosa controlla); bottone **Validate** che mostra i problemi di config (`engine.Validate`) senza eseguire i check. Le descrizioni dei moduli sono ora in `internal/moduledoc`, condivise tra CLI (`explain`) e GUI (con test anti-drift). Fix: `.drawer` con `display:flex` ignorava l'attributo `hidden` (drawer sempre aperto) → aggiunto `.drawer[hidden]{display:none}`. Binding `Explain`/`Validate` testate.

## 0.57.0

- Desktop (CF-61, M16): l'export supporta **tutti i formati** — Markdown, JSON, HTML, JUnit, Prometheus, OTLP — via un selettore + bottone Export con file-dialog nativo (riusa i renderer `internal/output`). Helper `renderReport` testato.

## 0.56.0

- Firma & SBOM (CF-56): le release ora includono **SBOM** per archivio (syft) e firme **cosign keyless** (Fulcio/Rekor via OIDC di GitHub) su `checksums.txt` e sulle immagini Docker. goreleaser `sboms`/`signs`/`docker_signs`; `release.yml` con `id-token: write`, `cosign-installer` e `download-syft`. Provenienza verificabile con `cosign verify`/`verify-blob` (comandi in `installation.md`). **Chiude M14 (distribuzione & supply-chain).**

## 0.55.0

- Immagine Docker (CF-55): immagine **multi-arch** (linux/amd64+arm64) pubblicata su GHCR (`ghcr.io/allan-nava/checkfleet`) a ogni release, via goreleaser (`dockers` + `docker_manifests`). Base **distroless static nonroot** (CA incluse), `Dockerfile` minimale con l'**exporter** come entrypoint di default (`serve` su :9876). `release.yml`: QEMU + buildx, login a GHCR con `GITHUB_TOKEN`, permesso `packages: write`. Config validata con `goreleaser check`.

## 0.54.0

- Lint & vuln in CI (CF-57): nuovo job `lint` in `ci.yml`. **`govulncheck` come gate** (con Go `stable`: fallisce solo sulle vulnerabilità effettivamente raggiungibili dal codice; le vuln stdlib si risolvono tenendo Go aggiornato, quelle nelle dipendenze non-chiamate non bloccano). **`golangci-lint` advisory** (`continue-on-error`, non blocca) — si promuove a gate togliendo `continue-on-error` quando il tree è pulito. Docs in `development.md`.

## 0.53.0

- Export OTLP (CF-30): `--output otlp` emette una richiesta OTLP/HTTP **metrics** in codifica JSON — gli stessi gauge del formato `prometheus` (`checkfleet.finding.status`, `.findings.total`, `.worst.status`) — costruita a mano **senza dipendenze** (niente SDK OpenTelemetry). Si POSTa al `/v1/metrics` di un collector. `output.OTLP` testato. **Chiude M7 (alerting & output).**

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
