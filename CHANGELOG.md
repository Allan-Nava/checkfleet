# Changelog

## 1.15.0

- **`engine.PostProcess`: una pipeline sola dopo la run, e il test che impedisce di ri-divergere (CF-163, M36).** `check`, `serve` e `watch` applicavano a mano `ApplyMaintenance` → `ApplyRunbooks`; il desktop applicava **solo la seconda**. Conseguenza concreta, non teorica: **una finestra di manutenzione attiva silenziava la CLI e non la GUI**, quindi lo stesso `checkfleet.yml` dava due verdetti diversi a seconda di dove lo aprivi. Ora i quattro punti chiamano una funzione sola.

  **Cambio di comportamento nel desktop**: le finestre di manutenzione ora valgono anche lì. Se usavi la GUI per vedere *tutto* mentre la CLI silenziava, ora vedi quello che vede la CLI — che è il punto, ma va saputo.

  L'ordine dentro la pipeline non è casuale ed è documentato: **prima si silenzia, poi si annota**. Annotare per primo spenderebbe lavoro su righe che nessuno vedrà e, peggio, farebbe viaggiare il runbook di un finding silenziato dentro un sink che guarda solo gli hint.

  **Il valore non è la deduplicazione, è `TestPostProcessIsTheOnlyPipeline`.** Il test fa il parsing dell'AST dei sorgenti di `cmd/checkfleet` e `desktop/` e fallisce se uno di essi chiama un passo della pipeline direttamente invece di passare da `PostProcess`. Verificato che sappia fallire reintroducendo esattamente la deriva appena chiusa — rimesso `ApplyRunbooks` a mano in `desktop/app.go`, il test lo nomina e fallisce. Più `TestPipelineStepsAreAllRegistered`, che tiene onesta la lista dei passi: un passo aggiunto a `PostProcess` e non registrato renderebbe il gate cieco proprio ai bypass di quel passo. E un conteggio minimo di file esaminati, perché una walk che non guarda niente passa sempre.

  Questo item apre M36 e viene prima dei binding desktop di proposito: senza, la milestone avrebbe aggiunto sette occasioni nuove di divergere invece di chiuderne una.

- **Docs (backlog): nuovo item CF-173** in M36 — le analisi di M30 nell'output di `check`. Cinque delle sette vivono solo nel comando `insight`, e `Correlate`/`FleetScore` escono solo in markdown mentre il JSON è la superficie su cui si fa gating. Va fatto dopo CF-163 e prima dei binding, perché il desktop consumerà le stesse struct.


## 1.14.1

- **Docs (backlog, planning): nuova milestone M36 — parità CLI/desktop e insight nella GUI.** Nessun cambiamento al software.

  **Prima, una correzione su M30.** I suoi otto item sono marcati chiusi, ma il preambolo prometteva che ogni analisi si affacciasse "in output *e nel desktop*", e i binding desktop non esistono: `grep -rn 'internal/insight' desktop/` non trova niente. Nella GUI sono arrivati solo il drawer di CF-124 e la riga di fleet score nel markdown. M30 ora lo dice in testa (`✅ (lato CLI)`) invece di lasciar credere che sia intera.

  **Il problema sotto è più generale delle sette analisi mancanti.** Le due interfacce sono derivate: `check`, `serve` e `watch` applicano a mano `ApplyMaintenance` → `ApplyRunbooks`, il desktop applica **solo la seconda**. Oggi lo stesso `checkfleet.yml` produce due verdetti diversi a seconda di dove lo apri — una finestra di manutenzione attiva silenzia la CLI e non la GUI — e nessun test se ne accorge. Per questo il primo item di M36 non è una feature ma `engine.PostProcess`, la pipeline condivisa più il **test di parità** che rende la deriva impossibile; e va fatto prima dei binding, altrimenti la milestone aggiunge sette occasioni nuove di divergere.

  Dieci item (CF-163..172): la pipeline condivisa, i binding Wails di `internal/insight`, e poi le sei superfici GUI che le analisi meritavano — tile del fleet health con il trend dell'indice, forecast e banda di baseline sul grafico metrica del drawer, cluster blast-radius ripiegabili nella tabella, card SLO burn rate, MTTR e outage in corso, drawer "what changed" inoltrabile ai sink. Più due code: il punteggio di flappiness col badge (resto di CF-120, che aveva la detection ma non la misura) e il consumer group nella suite d'integrazione, perché `kadm.GroupLag` è rimasto a 0% anche dopo CF-161 — la copertura è salita, quel percorso no.


## 1.14.0

- **Una pagina per modulo, generata (CF-147, M33).** `go run ./cmd/gen-docs` produce le **29** pagine `docs/modules/<name>.md` da `docs/_data/modules.yml` (id, titolo, summary) e dalle sezioni già scritte in `docs/modules.md`. Motivo: una pagina da 600 righe compete per un solo termine di testa, mentre il traffico vero è la coda lunga — "checkfleet kafka consumer lag check", "check hls live edge cli" — dove una pagina dedicata vince senza competere con niente. La pagina combinata resta l'indice, e ora linka le singole.

  **Nessuna prosa riscritta a mano.** Le pagine sono una *proiezione* di `modules.md`: la prosa ha una casa sola. Un modulo presente nell'indice ma senza sezione fa **fallire il generatore** con il nome del modulo, invece di pubblicare una pagina vuota — e la CI rigenera e fallisce sul diff, stesso gate dei reference della skill. Verificato che sappia fallire: sporcando `kafka.md` a mano, `-check` esce 1 nominando il file.

  **Difetto nel mio primo controllo, non nel codice**: contando le sezioni con `[a-z]+` ne trovavo 28 contro 29 e sembrava che `modules.md` avesse perso un modulo. Era la regex a perdere **`s3`**, per via della cifra. Le tre sorgenti (registry, `modules.yml`, `modules.md`) erano già allineate — ma il generatore usa `[a-z0-9]+` e c'è un test dedicato a `s3`, perché un pattern che scarta le cifre in silenzio non genererebbe mai quella pagina e nessuno se ne accorgerebbe.

  Le pagine sono `nav_exclude` (29 voci nella sidebar sarebbero peggio dell'indice) con `permalink: /modules/<name>`.


## 1.13.0

- **Digest "what changed" (CF-128, chiude M30).** `checkfleet insight --digest` riassume in prosa cosa si è mosso nella finestra, invece di far diffare le righe a mano:

  ```
  Across the last 100 run(s): 1 new problem(s), 1 flapping.

  New:
    - http https://web-01/: OK → BAD

  Flapping:
    - http https://api-01/
  ```

  Quattro movimenti — **nuovi, peggiorati, migliorati, risolti** — più chi ha iniziato a oscillare. La prima riga porta i conteggi, così il testo è già inoltrabile: ai sink di M22, in una issue, o in cima a un doc d'incidente.

  **Una flotta ferma lo dice in una riga** (`Nothing changed across the last N run(s)`) invece di stampare quattro intestazioni vuote. Sembra un dettaglio: è la differenza fra un digest che si legge ogni mattina e uno che si smette di aprire.

  **Uno stato sconosciuto non diventa mai un problema.** Un record scritto da una build più recente porta uno status che questa non sa ordinare: viene trattato come `OK` nel confronto, invece di essere letto come regressione. Un digest che inventa guasti dopo un upgrade parziale della flotta è peggio di nessun digest, e il test lo blocca.

  **M30 chiusa.** Gli otto item: `internal/insight` e il forecast ETA (CF-121), anomaly EWMA/z-score (CF-122), correlation e blast-radius (CF-123), runbook e remediation hint (CF-124), SLO error budget e burn rate (CF-125), MTTR e outage in corso (CF-126), fleet health score (CF-127), e questo. Più CF-120, chiuso rileggendo il backlog contro il codice: la flapping detection che la milestone pianificava era già stata consegnata da CF-32.

  Tutte le analisi sono funzioni pure in `internal/insight`, zero-dep, con la statistica scritta a mano, e si combinano in una sola invocazione di `checkfleet insight` — che non tocca infrastruttura: legge solo il JSONL che `check --history` già scrive.


## 1.12.0

- **Fleet health score (CF-127, M30).** Un indice **0–100** per la flotta, con il breakdown per modulo che gli dà un posto dove puntare:

  ```
  $ checkfleet insight --history runs.jsonl --score
  Fleet health: 50.0/100 over 2 finding(s), 1 unstable target(s)
    http            50.0
  ```

  E una riga nel report markdown (`**Fleet health: 92.4/100**`), lì status-only: il renderer non ha la history, quindi il sovrapprezzo per instabilità resta un mestiere di `insight --score`.

  **I pesi sono esportati e documentati** (`StatusWeights`, `InstabilityWeight`): un punteggio la cui aritmetica è nascosta è un numero di cui nessuno si fida due volte, e a quel punto tanto valeva non averlo.

  Tre scelte che decidono se l'indice serve. **`ERROR` costa un po' meno di `BAD`** (4 contro 5): "non sono riuscito a misurare" è una brutta notizia sulla sonda e solo *forse* sul target, e pagarli uguale farebbe sembrare un blip di rete durante una run identico a un outage. **Un target che flappa paga un sovrapprezzo** oltre al suo stato: oscillare è peggio di un BAD stabile già triagiato, perché sveglia gente ripetutamente e nasconde le transizioni vere nel rumore. E il punteggio è **normalizzato sul peggio possibile per quella flotta**, quindi due flotte di dimensioni diverse con la stessa proporzione di guasti danno lo stesso numero — un indice che deriva col numero di target non si può guardare nel tempo, che è l'unica ragione per averne uno.


## 1.11.0

- **MTTR e durata dell'outage in corso (CF-126, M30).** `checkfleet insight --recovery` misura le risalite:

  ```
  http  https://api-01/   up · 2 outage(s), MTTR ~1h0m (p50 1h0m, p90 1h0m)
  http  https://web-01/   down for 7h3m
  ```

  Il numero che trasforma una riga rossa in una decisione non è la durata, è il **confronto**: "giù da 47m, di solito torna in ~8m" richiede un intervento diverso da "giù da 47m, di solito ci mette 2h". Quando il target è giù *e* ha uno storico, la riga porta entrambi.

  **Un outage iniziato al bordo della finestra è un limite inferiore, non una durata**, e viene detto (`down for at least 3h (started before the window)`): è cominciato prima dello storico che abbiamo, quindi riportarne la durata come fatto la sottostimerebbe — sistematicamente proprio sugli outage più lunghi, che sono quelli che contano.

  I percentili usano il **nearest-rank**: con tre outage il "p90" è il più lungo dei tre, che è la risposta onesta per un campione piccolo. Interpolare inventerebbe una precisione che tre misure non hanno.

  Stessa definizione di downtime del budget d'errore: `BAD` ed `ERROR` sì, `WARN` no. Un target che non è mai andato giù non compare — una riga che dice "0 outage" è rumore in un report che si legge durante un incidente.


## 1.10.0

- **SLO error budget e burn rate (CF-125, M30).** `checkfleet insight --slo 0.99` non dice quanto sei stato su, dice **quanto in fretta stai finendo il margine**:

  ```
  http  https://api-01/   98.00% up  80% of budget left (burn 0.20x)
  http  https://web-01/   92.00% up  20% of budget left, fast burn 8.0x → gone 2026-08-05T01:34
  ```

  Due finestre come da manuale SRE: la **slow burn** sull'intera storia e la **fast burn** sull'ultima frazione (`--fast-window`, default 0.1). Servono a distinguere i due guasti che una percentuale sola confonde — due blip vecchi che hanno consumato un quinto del budget senza urgenza, e un'ora di down in corso che se ne mangia il resto entro sera. La proiezione di esaurimento esce solo dalla fast burn, perché è il ritmo *attuale* la cosa su cui si decide.

  **`WARN` non è downtime.** Contare ogni warning come indisponibilità renderebbe impossibile per costruzione qualunque obiettivo sopra le due nove — è una di quelle scelte che, sbagliata, fa abbandonare la feature dopo una settimana. `BAD` ed `ERROR` sì: se il check non è riuscito a misurare, per l'availability non è "su".

  Niente budget sotto **dieci** run (con una manciata di campioni ogni blip sembra il 100% di burn rate), e `--slo` fuori da (0,1) è un errore esplicito invece di una divisione silenziosa.

  **Due difetti trovati dai test, entrambi tenuti.** Un budget speso *esattamente* usciva dalla divisione con ~1e-16 di margine residuo, cioè riportava "budget rimasto" su un target che l'aveva appena finito — ora c'è una tolleranza, e il verso dell'arrotondamento è quello prudente. E la mia prima aspettativa di test era sbagliata, non il codice: 1 fallimento su 100 run **sfonda** un obiettivo 99.9% di dieci volte, non lo intacca. Corretto il test, non l'implementazione.


## 1.9.0

- **Correlation / blast-radius: trenta righe rosse che sono un guasto solo (CF-123, M30).** `insight.Correlate` raggruppa i finding non-OK di una run per la dimensione che condividono — **host**, **subnet /24**, **modulo** — e il renderer markdown li mostra come sezione **ripiegabile** sopra la tabella completa:

  ```
  ## 🔗 Correlated failures
  <details><summary><b>3 failures</b> share the same host: <code>db-01</code></summary>
  ```

  Un host morto produce un finding per ogni modulo che lo tocca. Leggerli come eventi separati è il modo in cui un blast radius sparisce nello scroll: la domanda non è "quali check sono rossi" ma "cos'è successo".

  **Ogni finding finisce in un cluster solo, e vince la dimensione più specifica** (host, poi subnet, poi modulo). Riportare lo stesso guasto sotto tre intestazioni sarebbe lo stesso muro di righe con passaggi in più — un test lo verifica contando che la copertura totale sia esatta e che nessun finding compaia due volte.

  Sotto **tre** membri non è un pattern: due finding sullo stesso host li leggi direttamente dalla tabella, e una sezione che si apre per quello è rumore. Senza gruppi la sezione non compare affatto.

  L'estrazione dell'host regge le forme che i moduli producono davvero — `db-01:5432`, `https://user:pw@a.example/health`, `db-01:5432/connections` (le sotto-metriche di postgres) — e rinuncia invece di indovinare. La subnet raggruppa **solo IPv4 letterali**: risolvere un nome per raggrupparlo significherebbe fare DNS dentro un'analisi offline, che è un effetto collaterale che un insight non deve avere.


## 1.8.0

- **Anomaly detection: deviazione dalla propria normalità (CF-122, M30).** `checkfleet insight --anomaly` confronta l'ultimo campione di ogni metrica con la **sua** baseline recente, non con una soglia statica. È la differenza fra "90ms sotto il limite di 500ms, tutto bene" e "90ms su un target che sta a 30ms da una settimana, guardaci".

  ```
  $ checkfleet insight --history runs.jsonl --anomaly
  http   https://a.example    95.00ms  3.2x its norm of 30.15ms (z=+48.0)
  redis  cache-01            100.00MB  normal (baseline 100.15MB)
  ```

  Baseline **EWMA** (α 0.3) con varianza incrementale, così il comportamento recente pesa più di quello del mese scorso, e il tutto resta zero-dep — media e varianza a mano in una decina di righe. Soglia configurabile con `--z` (default 3), e la deviazione è **segnata**: un crollo del throughput vale quanto uno spike della latenza, e il test lo verifica.

  **Tre modi in cui questa feature produce rumore, chiusi.** Sotto **sette** campioni non c'è baseline: con due letture "la norma" non esiste e una lettura su tre sembra anomala. Una metrica **perfettamente ferma** ha deviazione zero, quindi lo z-score sarebbe infinito: invece di dividere per zero e dichiarare ogni movimento infinitamente anomalo, quel caso resta a Z finito e il segnale utilizzabile diventa il **rapporto** (`1.8x la norma`), che è anche la formulazione più leggibile delle due. E ogni riga senza verdetto dice perché.

  `--forecast` e `--anomaly` si combinano nella stessa invocazione, e in `--output json` finiscono in due array distinti.


## 1.7.0

- **`internal/insight` e il forecast ETA-to-threshold (CF-121, M30).** Nasce il package che M30 aspettava: funzioni pure sopra la history, zero-dep, statistica scritta a mano — il punto della regola zero-dep è che un binario di monitoring che giri ovunque non si porti dietro uno stack numerico per due regressioni. Ci sono `SeriesFrom`/`StatusSeriesFrom` (raggruppano i record per `check`+`target`, ordinano i punti, ignorano i finding senza metrica) e la prima analisi.

  **Il forecast** fa ai minimi quadrati la retta della serie e proietta quando taglia una soglia — generalizzando l'ETA che oggi hanno solo i certificati: "disk 82% → sfonda 90% tra ~2.5 giorni" è la stessa domanda di "questo cert scade fra 12 giorni", fatta a qualunque metrica che checkfleet già registra. Emette slope, **R²** e la data.

  Nuovo comando, che non tocca infrastruttura (legge solo il JSONL che `check --history` già scrive):

  ```
  $ checkfleet insight --history runs.jsonl --forecast --threshold 95
  postgres   db-01     88.00%  crosses in ~3.5 days (+2.00%/day, R²=1.00)
  ```

  **Le tre cose che questa feature sbaglia se scritta ingenuamente, e che qui non sbaglia.** Con meno di **quattro** campioni non proietta niente: due punti fittano una retta perfettamente (R²=1) e non dicono nulla sul trend, che è esattamente la forma della sciocchezza detta con sicurezza. Una serie **piatta o in allontanamento** non riceve ETA, invece di riceverne uno assurdo. E `--min-r2` (default 0.7) **sopprime** la proiezione quando il fit è debole, dicendo *perché* invece di tacere.

  **Difetto trovato provando il comando su dati veri**, non nei test: una serie che sale verso la soglia ma con campioni vecchi produce un crossing *nel passato*, e il codice lo riportava come "not trending toward the threshold" — cioè diceva "nessun rischio" proprio sul target più vicino alla linea. Ora `Forecast.Due` distingue i due casi e il messaggio è "trend says it should already be over the threshold (history may be stale)". I test sintetici non lo avevano preso perché passavano `now` coerente con la serie; è emerso solo con una history datata.

  Ogni riga senza ETA porta la ragione (`too few samples`, `not trending`, `weak fit`, `already over`): un campo vuoto si legge come "tutto bene", ed è l'errore che questa milestone deve evitare.


## 1.6.0

- **Gate anti-divergenza sulla skill e pagina `docs/agents.md` (CF-152, chiude M34).** La CI ora rigenera i reference e **fallisce se il diff non è vuoto**, stessa logica di un check `go generate`. È la garanzia che rende la skill affidabile invece che plausibile: un modulo nuovo non può entrare lasciando la skill indietro. La trappola non è ipotetica — è quella che ha lasciato l'intro di `docs/modules.md` ferma a 18 moduli mentre il registry ne aveva 29.

  Verificato che il gate sappia fallire, non solo passare: sporcando `references/modules.md` a mano, `gen-skill -check` esce 1 nominando il file; ripristinato, esce 0.

  Nuova pagina **`docs/agents.md`**: come installare la skill (globale, non nel repo, e da rifare dopo un upgrade), cosa contiene e perché è piccola, e i tre meccanismi che la tengono vera — reference generati, gate CI, e il test che compila il binario per verificare che ogni comando e flag citato esista.

  **E la nota su MCP**, che l'item chiedeva esplicitamente. Non ora, con le ragioni scritte invece che sottintese: checkfleet non tiene stato fra le chiamate e non fa streaming, cioè le due cose per cui MCP è la forma giusta. È un binario che prende una config e stampa un documento — cosa che un tool di shell espone già bene, e che ogni assistente sa eseguire mentre il supporto MCP varia. Un server aggiungerebbe un processo da supervisionare, un transport da debuggare e una seconda superficie da mantenere compatibile, in cambio di niente che la CLI non dia già. Se cambia il presupposto (stato di flotta persistente, subscription sulle transizioni di stato) si riconsidera nel merito.

  **M34 chiusa.** I quattro item: skill sorgente versionata col codice (CF-149), reference generati da registry e struct di config (CF-150), `checkfleet skill install/print` con la skill embeddata (CF-151), e questo.

## 1.5.0

- **`checkfleet skill` — la skill viaggia dentro il binario (CF-151, M34).** `go:embed` porta `skills/checkfleet/` (SKILL.md più i due reference generati) dentro l'eseguibile; `checkfleet skill install [--dir PATH]` la scrive dove serve — di default `~/.claude/skills/checkfleet/` — e `checkfleet skill print` la manda su stdout per chi ha un installer suo.

  Il punto non è la comodità: **chi ha il binario ha la skill alla versione giusta**. Tutto il valore della skill sta nel citare flag che esistono davvero nel binario che stai eseguendo, e una skill installata una volta da un repo clonato mesi prima è precisamente la cosa che quel valore lo distrugge. `TestEmbeddedSkillMatchesTheSource` verifica che i byte nel binario siano i byte nel repo.

  Il file che fa l'embed sta in `skills/embed.go`, accanto alla sorgente: `go:embed` non può risalire sopra la directory del proprio package, e l'alternativa era una **seconda copia di SKILL.md** sotto `internal/` che sarebbe divergita dalla prima — lo stesso difetto che CF-150 esiste per evitare.

  L'install **sovrascrive**, di proposito e in modo idempotente: dopo un upgrade i file vecchi devono sparire, altrimenti binario e skill litigano su quali flag esistono. Tre test coprono le tre cose che possono andare storte lì (albero scritto, due install = tre file non sei, versione vecchia rimpiazzata). Semantica dell'exit code rispettata: non è un check, quindi 0 salvo errore sistemico — verificato sul binario reale, `print` esce 0 e un subcomando ignoto esce 1 con l'errore che nomina cosa era valido.

  **Difetto trovato aggiungendo il comando**: la lista dei subcomandi nello script di completion era **già stale** — `init`, `alert`, `doctor` e `targets` erano stati rilasciati senza mai diventare completabili, e `skill` si sarebbe aggiunto alla lista dei dimenticati. Corretta, e aggiunto `TestCompletionListsEverySubcommand` che la confronta con l'usage vero invece che con una copia. Il test conta quanti comandi ha esaminato e fallisce se sono meno di dieci: senza quel controllo il loop poteva non guardare niente e passare sempre, che è il modo in cui muoiono i test che leggono la documentazione.

## 1.4.0

- **Reference della skill generati dal codice (CF-150, M34).** I due file che `SKILL.md` carica on-demand — `references/modules.md` (29 moduli, cosa rileva ciascuno) e `references/config-schema.md` (chiavi, tipi, default) — ora li scrive `go run ./cmd/gen-skill` invece di una mano umana. La ragione è concreta e già successa in questo repo: **l'intro di `docs/modules.md` è rimasta ferma a 18 moduli mentre il registry ne aveva 29**. Una copia mantenuta a mano diverge alla prima release, e lo schema della config è esattamente la parte su cui un assistente inventa di più.

  Le sorgenti sono quelle che usa già il binario in produzione: `internal/registry` per l'elenco dei moduli (lo stesso su cui `check` fa dispatch) e `internal/moduledoc` per le descrizioni (le stesse che stampano `checkfleet explain`, le rule SARIF e il desktop).

  **I default non sono letti dai commenti.** Il generatore carica via `engine.LoadBytes` una config che abilita ogni modulo — il che fa applicare i default veri — e poi riflette sul risultato. Quindi `timeout_seconds: 30`, `warn_days: 30`, `crit_days: 7`, `port: 443` nella tabella *sono* i valori che il codice applica, e un default che cambia cambia qui senza che nessuno se lo ricordi. Un test blocca proprio quelle quattro righe: se `applyDefaults` cambia, lo si scopre lì.

  Aggiunta una sezione **Referenced types** che espande `PostgresTarget`, `HTTPTarget` e gli altri: lasciare `CertsTarget` come token opaco è precisamente ciò che porta un assistente a inventarsi i nomi dei campi.

  Sette test: determinismo (cinque giri per file — l'ordine d'iterazione delle mappe è il modo tipico in cui si rompe, e romperlo farebbe fallire il gate CI di CF-152 a ogni run senza motivo), copertura di **ogni** modulo del registry in entrambi i file, ogni modulo ha una descrizione, i default reali, l'espansione dei tipi target, nessun segreto, e che la config di partenza abiliti davvero tutti i moduli invece di saltarne in silenzio.

  `go run ./cmd/gen-skill -check` esce non-zero se i file su disco sono stale — è l'aggancio per il gate di CI.

## 1.3.0

- **Skill `checkfleet` per gli assistenti agentici (CF-149, M34).** Sorgente in `skills/checkfleet/SKILL.md`, versionata col codice e installata **globalmente** dall'utente — non repo-local: checkfleet si usa *dagli altri* repo e dagli host, mentre qui dentro c'è già `CLAUDE.md` che copre lo sviluppo.

  Il corpo resta a **3.9 KB** contro un cap di 6 KB imposto da un test, perché il budget di contesto è la risorsa scarsa e tutto ciò che è sempre caricato lo consuma. Dentro c'è solo quel che un agente sbaglia senza: i comandi, le **due regole di semantica**, e il flusso JSON.

  Le due regole sono la ragione per cui la skill esiste. **Exit code 0 non significa "tutto sano"**: un check che gira è un successo anche se trova un BAD, e per far fallire una pipeline serve `--exit-on bad` esplicito — un agente che deduce salute dall'exit code riporta il contrario del vero. **`ERROR` non è `BAD`**: BAD è "il target è malato", ERROR è "non sono riuscito a misurare", e leggere "il database è giù" da un ERROR è un'affermazione che i dati non sostengono. Terza cosa: **leggere `--output json` e il campo `worst`**, non grepare il testo, la cui formulazione il contratto dichiara esplicitamente instabile.

  La `description` nomina i trigger veri (certificati in scadenza, i nomi dei moduli, il gating in CI) invece di parafrasare il nome — una descrizione tipo "la skill di checkfleet" non matcha nessuna domanda reale, e un test lo verifica.

  **Sei test, di cui uno che verifica il verificatore.** Front matter valido con `name: checkfleet`; dimensione sotto il cap; nessun segreto letterale; le due semantiche presenti; e il gate anti-finzione: ogni subcommand e flag che la skill mostra come eseguibile deve esistere davvero nell'usage del binario — compilato ed eseguito, non una copia. Una skill che cita con sicurezza un flag rinominato è peggio di nessuna skill, perché l'agente continua a riprovarlo e dà la colpa all'ambiente.

  Il gate ha trovato un caso al primo giro (`can checkfleet see X?` in prosa letto come subcommand `see`). Risolto **restringendo l'analisi a fence e span di codice** invece di allargare una lista di eccezioni: l'agente esegue quello, non la prosa, e una lista di eccezioni sarebbe cresciuta a ogni frase nuova finché il gate non controllava più niente. Aggiunto `TestSkillGateCatchesAFakeFlag`, che dimostra che il controllo sa fallire — un validatore che nessuno ha visto rifiutare qualcosa non è un validatore.

  I due file `references/` che la skill cita sono generati da CF-150.

## 1.2.0

- **mysql, mongodb e kafka nella suite d'integrazione (CF-161, chiude M9-bis).** I quattro moduli col driver vendorizzato (`mysql/driver.go`, `postgres/pgx.go`, `mongodb/mongo.go`, `kafka/kadm.go`) hanno un pezzo che un unit test non può raggiungere: l'adapter parla un wire protocol, quindi o c'è un server vero o non lo si esercita — e falsificare il protocollo a mano sarebbe più codice non testato di quello che copre. Postgres era già nello stack; gli altri tre no. Ora ci sono: **mysql 8.4**, **mongo 7** e **kafka 3.8 in KRaft** (single node, niente ZooKeeper), ciascuno con healthcheck così `--wait` resta deterministico, più i target in `checkfleet.integration.yml` e tre test che asseriscono reachability come gli altri.

  **Verificato eseguendolo, non assumendolo.** Stack su, suite verde:

  ```
  mysql   [OK] mysql-integration: reachable, MySQL 8.4.11 (read-write)
  mongodb [OK] mongo-integration: reachable, MongoDB 7.0.39 (standalone)
  kafka   [OK] cluster: 1 brokers, controller present
  ```

  E il profilo di copertura dice esattamente cosa sbloccava. Offline i test toccano **solo i rami d'errore** degli adapter — `driverConnect` 75%, `mongoConnect` 53.8% — mentre `Collect`, `Close`, `statusInt`, `variableInt`, `replicaStatus`, `showStatusRow` e **tutto `kadm.go`** stanno a **0%**: nessun unit test arriva oltre la connessione fallita. Con la suite quei simboli si accendono (`Collect` 84.6% su mysql, 62.5% su mongodb, `kadm.Metadata` 45.5%). `mysql` resta il modulo con la copertura unit più bassa (47.4%) e la ragione è questa, scritta in `docs/development.md` invece che nascosta.

  Resta a 0% `kadm.GroupLag`: la config d'integrazione non dichiara consumer group, quindi il lag non ha niente da misurare. Detto qui perché un numero di copertura che sale non è la stessa cosa di un percorso coperto.

  `timeout-minutes` del workflow da 25 a 35 — l'immagine kafka è la più lenta a diventare healthy (bootstrap KRaft).

## 1.1.1

- **I test del frontend desktop ora girano in CI (CF-162).** I sei file `desktop/frontend/*.test.js` — 48 asserzioni su mute, note, action log, grafici, preset e i runbook hint appena aggiunti — esistevano dal CF-112 e **nessuno li eseguiva mai**: `desktop-test.yml` faceva `node --check desktop/frontend/dist/main.js`, cioè un parse della sintassi di un solo file. È la stessa trappola che CF-157 aveva trovato sul lato Go (test end-to-end che non contribuivano copertura), qui in forma più netta: non è che contassero poco, è che non partivano. Aggiunto lo step `node --test`, ed esteso `node --check` da `main.js` a tutti i moduli in `dist/` — `runbook.js` era già il sesto file mai controllato.

  Corretta anche l'invocazione scritta negli header dei test: dicevano `node --test desktop/frontend/`, che da **Node 22** non risolve più una directory e fallisce con `MODULE_NOT_FOUND`. Ora dicono la forma che funziona (`node --test *.test.js` dalla directory), che è anche quella usata dal workflow — così la riga nel commento e la riga in CI non possono divergere.

## 1.1.0

- **Runbook e remediation hint sui finding (CF-124, M30 — insight & intelligence).** Un finding dice *cosa* è rotto; da ora la config può dire anche *cosa farci*. La nuova chiave `runbooks:` è una lista di regole che matchano come le finestre di manutenzione — glob su `check` e `target`, vuoto = tutti — e attaccano al finding un `runbook` (URL della procedura) e una `remediation` (nota breve). Chi è di turno legge il BAD e ha già il link, invece di andare a cercare la pagina del wiki.

  ```yaml
  runbooks:
    - check: certs
      runbook: https://wiki.example.com/runbooks/tls-renewal
      remediation: Renew with certbot, then reload haproxy
    - check: postgres
      target: "db-*"
      remediation: Check replication lag before failing over
  ```

  **Il primo valore non vuoto vince per campo, non per regola**: una regola specifica può fornire il runbook e una catch-all sotto di lei riempire la remediation che la prima lascia vuota. È la differenza che rende utile una catch-all invece di costringere a ripetere l'URL su ogni modulo.

  **Solo sui finding sopra `OK`**: su un risultato verde non c'è niente da fare, e ripetere l'URL su ogni target sano è rumore in ogni output — in una flotta grande è la maggior parte delle righe. Un consumatore deve quindi trattare i due campi come assenti su qualunque finding, non solo su quelli non configurati; scritto in `docs/compatibility.md`.

  Dove si vedono: riga indentata `↳ nota — url` in **text**; seconda riga nella cella Detail di *Needs attention* in **markdown**, con il runbook come link; campi `runbook`/`remediation` in **json** (omessi quando assenti); riga smorzata sotto il messaggio in **html**; blocco **What to do** nel detail drawer del **desktop**, che apre l'URL nel browser di sistema. La tabella markdown resta a quattro colonne — la sua forma è una superficie documentata, quindi l'hint viaggia dentro la cella esistente invece di aggiungerne una.

  Il campo JSON è additivo e `omitempty`, quindi **nessun bump di `schema`**: il contratto dice esplicitamente che aggiungere un campo non lo merita. Applicato in `check`, `serve`, `watch` e nel desktop, sempre subito dopo `ApplyMaintenance`, così un finding dice la stessa cosa ovunque.

  **Sicurezza.** L'URL arriva dalla config dell'operatore ma finisce dentro un `href` nel report HTML e nel drawer, e quel report si incolla nei doc d'incidente: solo `http(s)` diventa cliccabile, qualunque altra cosa (`javascript:` in testa) viene resa come testo inerte. Testato in entrambi i renderer, insieme all'escaping di virgolette nell'attributo. E una nota in evidenza in `docs/configuration.md`: qui va testo operativo, mai una credenziale — questi campi viaggiano in ogni output, compresi quelli che escono dall'host (Slack, webhook, issue tracker).

  **Difetto trovato scrivendo i test**: `TestFormatsAreDocumented`, il gate anti-divergenza che impedisce a un formato di cambiare senza aggiornare il contratto, iterava **solo le chiavi top-level** del JSON. Le chiavi *dentro un finding* sono annidate, quindi un campo nuovo per-finding poteva atterrare non documentato — esattamente ciò che quel test esiste per fermare, e `value`/`unit` erano passati di lì. Ora controlla anche quelle.

  Test: sei sull'`ApplyRunbooks` puro (match per check/target, `OK` saltati, primo-non-vuoto per campo, regola inerte, input non mutato), cinque sui renderer, sei headless sul modulo desktop `runbook.js`, più il contratto aggiornato con un finding che porta gli hint e uno che porta la metrica — così l'`omitempty` di entrambe le coppie resta verificato.

- **CF-120 chiuso come già fatto.** Rileggendo il backlog contro il codice: la *flapping detection* che M30 pianificava era già stata consegnata da **CF-32** (`history.Flaps()`, `--flap-changes`/`--flap-window`, finding `flap` WARN, testato) quando chiudeva M8. M30 l'aveva ripianificata da zero. Resta scoperto solo ciò che CF-32 non prometteva — un punteggio di flappiness invece del conteggio secco, e il badge nel desktop — annotato nel backlog come tale invece di restare un item aperto che sembra intero.

## 1.0.0

- **1.0.0.** `v1.0.0-rc.1` è promossa a stabile senza modifiche: dall'rc non è cambiata nessuna delle sette superfici del contratto di compatibilità, quindi non c'è un rc.2 da fare prima. Da qui in poi vale ciò che `docs/compatibility.md` promette — schema della config, chiavi JSON, nomi delle metriche Prometheus, exit code, identità dei finding, ordinamento worst-first e formato dei file di history/baseline non cambiano significato dentro la 1.x, e una rimozione passa per la deprecation policy.

  Lo scope resta quello dichiarato nell'rc: **il 1.0 copre la CLI**. L'app desktop resta beta con versione propria, per le ragioni scritte in `docs/desktop.md` (build macOS non firmata né notarizzata, nessuna icona nella menu bar fino al port a Wails v3).

  Nessun cambiamento al software in questo tag — è la promozione dell'rc. Le milestone additive rimaste (M30 insight, M34 skill per gli agenti) partono dalla 1.1.

## 1.0.0-rc.1

- **Release candidate della 1.0** (CF-160, chiude M35). Due decisioni di scope prese invece di rimandate, più il processo per arrivare al tag.

  **Il 1.0 copre la CLI.** Il binario, lo schema della config, gli output e gli exit code: le sette superfici del contratto di compatibilità (v0.138.0). L'**app desktop resta fuori**, beta e con versione propria — e le ragioni sono concrete, non prudenza generica: il build macOS **non è firmato né notarizzato** (Gatekeeper avvisa al primo avvio) e non c'è **icona nella menu bar** (Wails v2 non ha systray, arriva col port a v3). Promettere stabilità su un'app non firmata la cui finestra è ancora in ridisegno sarebbe una promessa fatta per essere rotta. Scritto in `docs/desktop.md` con una nota in evidenza e in `docs/compatibility.md`, così chi ha bisogno di garanzie sa che deve scriptare la CLI.

  **La licenza, detta in chiaro.** Il caso che tutti sbagliano è uno: *usare checkfleet in un'azienda for-profit è uso commerciale anche se resta puramente interno e nessun cliente lo vede mai* — "commerciale" nella PolyForm Noncommercial riguarda **lo scopo che il software serve**, non la ridistribuzione. Era deducibile da `COMMERCIAL.md`; ora è la prima cosa che quella sezione dice, più una **nuova voce della FAQ** (che nomina anche ciò che resta gratis: la valutazione, il chiedere se il proprio caso rientra, e tutto il non-profit/pubblico a prescindere dal budget) e un paragrafo nel README. È il fattore che più determina l'adozione: a 1.0 diventa definitivo nella percezione, quindi va letto senza interpretazione.

  **Perché un rc e non direttamente il tag.** Tutto quello che sta nel contratto è una promessa che sopravvive alla release che la fa, e uno schema che nessuno ha stressato è esattamente ciò che si scopre di aver sbagliato la settimana dopo aver promesso di non cambiarlo. Quindi: `v1.0.0-rc.1`, un periodo di uso reale su una flotta vera, poi `v1.0.0`. Se l'rc trova qualcosa, cambia nel prossimo rc — è a questo che serve. Sezione "Getting to 1.0" in `docs/compatibility.md`.

  **Fix trovato preparando l'rc**: `homebrew_casks.skip_upload` era `"false"`, quindi il tag dell'rc avrebbe **pubblicato il suo cask nel tap** e `brew install Allan-Nava/tap/checkfleet` avrebbe consegnato una release candidate a tutti — l'opposto di cosa serve un rc. Ora è `"auto"`: pubblica sui tag normali, salta le prerelease. La prerelease su GitHub resta, quindi l'rc è installabile *volendolo*, dall'archivio. Validato con `goreleaser check`.

  **M35 chiusa.** Gli otto item: contratto di compatibilità + versione dello schema (CF-153), chiavi ignote segnalate anche da `check` (CF-154), `SECURITY.md`/`CONTRIBUTING.md`/template (CF-155), split di `main.go` da 811 a 97 righe (CF-156), copertura CLI dal 17.3% al 64.5% (CF-157), guard su `moduledoc` e buchi nei moduli (CF-158), percorsi di distribuzione verificati con la causa del `brew-test` appeso (CF-159), e questo. Nessuna feature nuova in tutta la milestone, per costruzione: M30 (insight) e M34 (skill) restano additive e vanno in 1.1.

## 0.144.0

- Distribuzione (CF-159, M35 — verso la 1.0): il workflow `brew-test` **non verificava niente da mesi**, e il motivo è istruttivo. Il leg Intel della matrice chiedeva `macos-13`, immagine **ritirata il 2025-12-04**: un job che chiede un'etichetta senza runner dietro **non fallisce**, resta *queued per sempre*. Quindi il leg arm64 passava in pochi minuti e quello Intel restava appeso — run in coda da **13 a 22 ore**, workflow che non concludeva mai, e un tap che *sembrava* controllato senza che nessuno avesse mai davvero installato il cask. Un check verde-in-apparenza è peggio di nessun check. Ora la matrice usa **`macos-15-intel`** (l'ultima immagine x86_64 di GitHub, disponibile fino ad **agosto 2027**, dopo cui il ramo Intel del cask va verificato a mano — nota scritta nel workflow perché è una scadenza che tornerà) più un `timeout-minutes` come guardia contro un install che si blocca su un prompt interattivo; le 9 run appese sono state cancellate. E poi la verifica che l'item chiedeva, fatta **a mano su v0.142.0** invece che assunta: lo `sha256` del cask coincide con quello dell'asset pubblicato, `checksums.txt` verifica, l'archivio `darwin_arm64` si estrae e il binario **gira riportando la versione vera** (`checkfleet 0.142.0`, non `dev` — quindi l'iniezione via ldflags funziona sull'artefatto reale, non solo in locale), l'immagine GHCR è **multi-arch amd64+arm64**, `:latest` risolve (la race di CF-67 non è tornata) e dentro il container un `check` produce i finding attesi. Infine le firme: **`cosign verify-blob` su `checksums.txt` e `cosign verify` sull'immagine danno entrambi `Verified OK`**, con identità legata a `release.yml@refs/tags/v0.142.0` — cioè la catena keyless che le docs promettono regge davvero. Un dettaglio trovato eseguendo i comandi documentati: le versioni recenti di cosign stampano un avviso di **deprecazione** per `--certificate`/`--signature` (la forma nuova è `--bundle`); il comando verifica ancora correttamente, e `docs/installation.md` ora lo dice invece di lasciare l'utente a chiedersi se l'avviso significhi che la firma non è valida.

## 0.143.0

- Test (CF-158, M35 — verso la 1.0): copertura dove il codice è contrattuale; totale del repo a **75.8%**. **`internal/moduledoc` da 0 a 100%** ed è l'aggiunta che conta di più, perché non è una percentuale: è una mappa **scritta a mano** che alimenta `checkfleet explain`, le **descrizioni delle rule SARIF**, i chip del desktop e (in M34) la skill — cioè il tipo di file che va stale nel momento esatto in cui si aggiunge un modulo, la stessa trappola che aveva lasciato l'intro di `docs/modules.md` a 18 moduli su 29. Il guard va in **entrambe le direzioni**: ogni modulo del registry deve avere una descrizione non vuota (altrimenti `explain` non dice niente e la sua rule SARIF esce senza descrizione), e ogni descrizione deve corrispondere a un modulo che esiste ancora (una doc orfana è una promessa che il binario non mantiene). Più un test anti-placeholder e uno anti-credenziali, dato che quel testo viene stampato e finisce nei report. Un tentativo di test più "intelligente" sulla *qualità* delle descrizioni è stato scritto e poi **rimosso**: falliva su 17 descrizioni perfettamente buone, ed è un promemoria che un'euristica sulla prosa produce solo falsi positivi. **`certs` 54.5 → 92.7%**: il buco non era la logica di probe (già testata) ma `Run`/`probeAll`, cioè l'espansione dei target e il fan-out concorrente — ora coperti con due server TLS locali a scadenza nota, asserendo che **l'ordine dei finding segue l'ordine dei target** e non quello in cui le goroutine hanno finito (un ordine racy qui renderebbe l'output non deterministico tra run), che un host da **inventory** viene sondato, e che un **inventory illeggibile produce un ERROR** invece di "niente da controllare" — che sarebbe una flotta sana dedotta da un refuso in un path. Per i quattro moduli col driver (`mysql`, `postgres`, `mongodb`, `kafka`) un test su `Run` verso una porta chiusa che assicura **ERROR e non BAD** — la regola che il contratto di compatibilità ora garantisce — più, per mysql, che il **DSN non finisca nel messaggio** del finding: `mongodb` 53→72.2%, `kafka` 47.8→62.3%, `postgres` 56.3→60.7%, `mysql` 31.6→47.4%. Il test mongodb usa un context con timeout esplicito: il driver ufficiale ha 30s di server-selection e una suite unit che ci mette mezzo minuto per scoprire che una porta è chiusa smette di essere lanciata. **Limite dichiarato invece che nascosto**: `mysql` resta sotto il 60% perché ciò che manca è **solo** l'adapter del driver, che parla un wire protocol e non è testabile senza un server vero — falsificarlo a mano sarebbe più codice non testato di quello che coprirebbe. È il caso per cui esiste la suite opt-in di CF-37, e `docker-compose.integration.yml` oggi non include mysql/mongodb/kafka: nuovo item **CF-161** per aggiungerli lì, dove l'adapter va coperto, senza tradire la regola che `go test ./...` resta offline.

## 0.142.0

- Test (CF-157, M35 — verso la 1.0): copertura di `cmd/checkfleet` da **17.3% a 64.5%**, e la scoperta che la spiegava: i test end-to-end esistenti **non contavano nulla**. Eseguono il binario in un sottoprocesso, quindi sono ottimi per asserire gli exit code ma la copertura non li vede — e la CLI è dove vivono flag, gate e fan-out, cioè tutto quello che il contratto di compatibilità promette di non rompere. Aggiunti quindi test **in-process** che chiamano le funzioni direttamente (`inprocess_test.go`, `forge_test.go`): `render` su tutti i 9 formati più il formato ignoto e il colore solo quando richiesto; `atomicWrite` che **sostituisce** e `appendFile` che **preserva** (`$GITHUB_STEP_SUMMARY` è condiviso con gli altri step del job: una rename butterebbe via il loro contributo); ogni sink push contro un webhook `httptest` con verifica che il payload sia JSON valido, più il caso **env non settata** che deve nominare *quale* variabile perché è tutta la fix; il template del webhook e il template mancante; `github` che scrive il job summary e che **fuori da Actions non è un errore**; l'**isolamento del fan-out** (un sink morto non fa fallire la run né ferma gli altri) contro il singolo sink che invece aborta; `pingDeadman` che usa `/fail` solo sul BAD; le tre transizioni del **baseline** (prima run che salta il gate, gate solo sui finding nuovi o peggiorati, `--baseline` senza `--fail-on-new` inerte); `recordHistory` con le metriche e il flapping; i comandi diagnostici; `runCheck` con i filtri e **sei errori sistemici** (modulo sconosciuto, config mancante, formato ignoto, `--diff` senza `--history`, `--fail-on-new` senza `--baseline`, `--exit-on` non valido). Per `report-issues` niente mock: un eseguibile **finto `gh`/`glab` messo in testa a `PATH`** che logga gli argomenti — così si verifica offline che una issue venga aperta, che la label sia creata prima, e che `--dry-run` **non tocchi niente**. Scrivendoli sono emersi due difetti veri, entrambi corretti: **`recordHistory` scartava `Value`/`Unit`**, quindi una history scritta dalla CLI non conteneva **nessuna serie metrica** mentre quella scritta dal desktop sì, dallo stesso formato documentato — il che avrebbe reso vuota la card "Metric over time" e ogni insight di M30 per chi usa `--history` da cron; e **`alert --dry-run` accettava un provider inesistente**, perché il nome era validato solo da `sendAlert`: l'errore arrivava *dopo* aver girato tutta la flotta, e sotto `--dry-run` non arrivava affatto — ora è controllato subito dopo il parsing dei flag, come già succede per `--exit-on`. Nessun test tocca la rete: dove l'endpoint è cablato nel codice (Telegram, PagerDuty, Opsgenie, SNS) il commento dice esplicitamente che la via d'invio è lasciata ai test dei renderer in `internal/`, perché un test che raggiungesse l'API reale sarebbe un bug, non più copertura.

## 0.141.0

- Refactor (CF-156, M35 — verso la 1.0): `cmd/checkfleet/main.go` era arrivato a **811 righe** con dentro il dispatch dei subcomandi, il parsing dei flag di `check`, il fan-out multi-sink, il gate, il rendering, la scrittura atomica, `serve`, `validate`, l'exporter e i poster HTTP. Un 1.0 congela anche il debito interno, e questo è il file che si tocca **ogni volta che si aggiunge un flag** — e i flag sono contratto. Spezzato per responsabilità in file che si leggono da soli: `check.go` (il comando `check` end-to-end), `emit.go` (**il dispatch dei sink** estratto dalla closure `emit` da 84 righe che viveva dentro `runCheck`, ora una funzione con `sinkOptions` che incorpora `renderCtx` — un renderer di formato ha bisogno di un sottoinsieme stretto, sono i sink push a servirsi anche dei nomi delle env var e del template), `render.go`, `sink.go` (poster HTTP + ping del dead-man's-switch), `serve.go`, `validate.go`, `watch.go`, `config.go` (flag e config → opzioni dell'engine). `main.go` resta **97 righe**: doc, `version`, `main` e `usage`, cioè solo il dispatch. Il file più grande ora è `check.go` a 244. **Zero cambiamenti di comportamento** — ed è stato verificato, non assunto: buildati i due binari (il vecchio da una worktree su `HEAD`) e confrontati su **10 formati di output** con una config deterministica offline (porta chiusa + dominio `.invalid`, nessuna rete) e su **14 invocazioni** che coprono i percorsi contrattuali — `--exit-on error`/`bad`, `--exit-code 42`, `--only`, `--min-severity`, `--target`, `validate`, `targets`, `doctor`, `explain`, `completion`, modulo sconosciuto, config mancante, formato inesistente. Output identico a meno dei valori di tempo (latenze, `started`, `timeUnixNano`) ed **exit code identici su ogni percorso**, `0`/`1`/`2`/`42` inclusi. Nessun test nuovo: la suite esistente è il guard, e un refactor che ne richiedesse di nuovi non sarebbe un refactor.

## 0.140.0

- Igiene di progetto (CF-155, M35 — verso la 1.0): i file che un repo pubblico a un passo dal 1.0 deve avere e non aveva. **`SECURITY.md`**: il canale è il **private vulnerability reporting di GitHub**, **abilitato sul repo** in questo giro (era `{"enabled":false}`) — il tasto "Report a vulnerability" nel tab Security invece di un'email in un file pubblico che i crawler indicizzano, e un report che resta privato fino alla fix. Il file dice cosa aspettarsi in tempi *dichiarati best-effort e non un SLA* (manutentore singolo: mentire su questo non aiuta nessuno), che le fix vanno solo sull'ultima release (è un binario statico: aggiornare = sostituire un file) e soprattutto delimita lo **scope** con le cose che per questo tool sono davvero vulnerabilità: **leak di credenziali** in output/log/report/issue/webhook (checkfleet maneggia DSN, token e webhook URL, e non stamparli è una regola di design — un leak è una vulnerabilità anche a severità bassa), injection via valori di config o risposte dei check, path traversal via `${file:...}`/`--out-file`, crash o memoria illimitata dai **parser di protocollo** che leggono input non fidato (ed è il motivo per cui sono fuzzati), e la supply chain della release. Altrettanto esplicito il **fuori scope**, in testa `InsecureSkipVerify` nel modulo `certs`: è voluto e documentato — leggere la scadenza di un certificato deve funzionare anche quando la chain non valida da dove sei, che è spesso tutto il punto — e verrà chiuso come by design; più "fare connessioni verso i target che hai configurato non è una SSRF, è la funzione del programma". Chiude con cosa il tool fa davvero con i segreti (solo da env, mai come flag dove finirebbero nella shell history; webhook e chiavi passati come *nome* della variabile; dei DSN viene reso solo host e porta), che è il contesto per giudicare un report. **`CONTRIBUTING.md`**: la parte onesta per prima (manutentore singolo, tempi best-effort, e il fatto che aprire una PR significa accettare che il contributo esca sotto la licenza del progetto **anche nelle copie commerciali** — nessun CLA, quel paragrafo è tutto l'accordo), il **gate reale con tutti e tre i comandi** incluso `golangci-lint` **v2** alla versione pinnata, le sette regole non negoziabili (test offline, semantica exit code, `ERROR` ≠ `BAD`, niente segreti, zero-dep di default, tutto in inglese, il contratto di compatibilità) e il **bar per un modulo nuovo**, che non è "si connette" — se un check `tcp` ti dice la stessa cosa, il modulo non si sta guadagnando il posto. Più **template issue** (bug report con versione/installazione/piattaforma, campo per l'output di `doctor` e una checkbox obbligatoria sulla redazione dei segreti — un bug report non deve essere la cosa che espone la flotta di chi lo apre; e un template "nuovo modulo" che chiede *quali stati non sani* e *perché contano* invece del nome del sistema), `config.yml` che dirotta le segnalazioni di sicurezza sul canale privato prima che diventino issue pubbliche, e **template PR** con il gate e le check come lista spuntabile. Corretta anche `docs/development.md`, che elencava il gate come "vet + test" omettendo proprio il linter che in CI è hard gate — la stessa lacuna che aveva lasciato passare 20 run rosse di fila; ora include la v2 con il comando di install e i test del modulo `desktop/`. README: sezione Contributing con i puntatori.

## 0.139.0

- Config (CF-154, M35 — verso la 1.0): le **chiavi ignote** smettono di essere un effetto collaterale e diventano una decisione scritta. Il parse resta **volutamente tollerante** — `KnownFields` non è attivo e non lo sarà: una config scritta per un binario più nuovo deve girare su uno più vecchio, ed è una garanzia del contratto di compatibilità, non una dimenticanza — ma tollerato non è la stessa cosa di nascosto. Finora un refuso lo nominavano solo `validate` e `doctor`, cioè solo a chi si ricordava di lanciarli: un `check` schedulato in cron continuava a **riportare una flotta sana** mentre un modulo scritto male non girava mai. È il peggior output che questo tool possa produrre — non una risposta sbagliata, una risposta sbagliata *sicura di sé* — quindi ora lo dicono anche i comandi che agiscono sulla config: **`check`, `serve`, `report-issues`, `alert`, `targets`** stampano `checkfleet: warning: unknown key "postgress" at \`checks\` — … → did you mean "postgres"?`. Tre scelte deliberate: la notifica va su **stderr** (mai dentro il documento renderizzato o il payload di un webhook — con `--output json` finirebbe nel JSON e lo renderebbe impossibile da parsare per la pipeline che lo consuma), **non cambia l'exit code** (un refuso non è un errore sistemico) e **non aborta la run**. Nuova `engine.UnknownKeys(path)`, che oltre a esporre il rilevamento già scritto per CF-132 **segue la catena degli `include`** — un buco reale del vecchio `Inspect`, che si fermava al file principale mentre il refuso in un drop-in di `conf.d` è più difficile da vedere proprio perché la config principale sembra a posto; i problemi dei file inclusi **nominano il file**, altrimenti chi legge non sa dove andare a guardare. Coperti anche gli overlay `--stack` (il file che si edita di fretta), con dedup delle righe perché base e overlay possono includere lo stesso drop-in. Un file mancante o non parsabile continua a non produrre nulla: sono errori del loader, e questa non deve diventarne una seconda copia peggio scritta. TDD: `internal/engine/unknown_test.go` (refuso di modulo con suggerimento, config pulita silenziosa, typo dentro un file incluso con il nome del file nel messaggio, `include` che non è una chiave ignota, file mancante, **ciclo di include** che non deve né appendere né ricorrere all'infinito) + `cmd/checkfleet/unknown_test.go` con stdout e stderr **separati**, che è il punto: chiave nominata su stderr con exit **0**, `--output json` che resta **parsabile** e col report intatto, config valida che non stampa niente, e refuso in un overlay di stack. Docs: sezione "Why misspelled keys matter most" in `docs/usage.md` estesa con il warning dei comandi non-diagnostici, e il punto sulle chiavi ignote in `docs/compatibility.md` ora descrive il comportamento reale invece di prometterlo.

## 0.138.0

- Compatibilità (CF-153, M35 — verso la 1.0): nuova pagina **`docs/compatibility.md`**, il documento che un 1.0 richiede e che non esisteva: dichiara **superficie per superficie** cosa resta stabile per tutta la 1.x, cosa esplicitamente non lo è, e con che policy di deprecazione. Le nove superfici stabili sono lo **schema della config** (chiavi documentate, più due proprietà che erano accidenti e ora sono contratto: le chiavi ignote non abortiscono la run — una config scritta per un binario più nuovo gira su uno più vecchio — e l'interpolazione `${...}`), il **JSON output** (chiave per chiave, con `worst` indicata come *il* campo su cui gatare), gli **exit code** con la regola portante che i finding non fanno fallire la run da soli, la **semantica degli status** (`ERROR` = non ho misurato, non "target malato"), l'**ordine dei finding** (worst-first, stabile, deduplicato), l'**identità di un finding** — la coppia `check`+`target`, che non è cosmetica: è la **chiave di dedup** di `report-issues`, `alert`, history e baseline, quindi rinominare un modulo o cambiare come scrive i suoi target riaprirebbe ogni issue e rifarebbe scattare ogni alert risolto —, i formati **history/baseline**, i **nomi delle metriche Prometheus** (incluso l'encoding numerico della severità, contro cui la gente scrive le alert rule) e i **nomi di comandi e flag**. Altrettanto importante la lista di cosa **non** è stabile, in testa **il testo dei messaggi dei finding**: migliora tra le release, e chi gata con un grep sul messaggio si rompe da sé — `worst` o `status`, mai un substring. Tecnicamente arriva il gancio che permette di evolvere senza rompere in silenzio: campo **`schema`** nel JSON output (`output.JSONSchemaVersion`) e **`sv`** su ogni riga del JSONL di `history` (`history.SchemaVersion`), stampato da `Append` così anche il desktop, che costruisce il `Record` a mano, lo ottiene senza modifiche. La lettura resta compatibile **in entrambe le direzioni**: un record scritto prima che il campo esistesse si legge come versione 1 (i file sono già sul disco degli utenti e li leggono `--diff` e `--fail-on-new`), mentre un record con versione **più recente** viene **saltato e segnalato** invece di essere interpretato come v1 — un diff sbagliato con sicurezza è peggio di un errore rumoroso, ed è l'unico caso in cui `Recent` restituisce i record leggibili *insieme* a un errore. Il baseline aveva già il suo `version` e resta com'è. TDD: `internal/output/contract_test.go` e `internal/history/contract_test.go` fissano gli insiemi di chiavi (JSON top-level, finding con e senza metrica — `value`/`unit` devono essere **assenti**, non `null` —, `Record`, `Entry`) e i nomi delle metriche, con il messaggio d'errore che rimanda al doc; più il test legacy-senza-`sv`, quello dello schema sconosciuto e **il gate anti-divergenza**: ogni chiave e ogni nome di metrica che i renderer emettono deve **comparire nella pagina**, così un cambio di formato non può entrare senza che il documento venga aggiornato nello stesso commit — la stessa classe di trappola che aveva lasciato l'intro di `modules.md` ferma a 18 moduli su 29. Docs: sezione `json` di `docs/output.md` con il campo `schema` e il perché `message` non è affidabile, riga sulla semantica in `llms.txt` (è la superficie che gli assistenti citano), link in README; la pagina entra da sé nella sidebar e in `llms-full.txt`, che le enumerano per `nav_order`.

## 0.137.1

- Docs (backlog, planning): aggiunta la milestone **M35 — Verso la 1.0 (fase 4)**. Il software è ampiamente oltre la soglia di un 1.0 (29 moduli, desktop, GitHub Action, release firmate con SBOM): quello che manca **non sono feature, è il contratto**. Un 1.0 non significa "abbastanza roba", significa la promessa *"da qui non ti rompo niente"*, e oggi quella promessa non è né scritta né tecnicamente preparata — le superfici che qualcuno già parsa e che il tag congela sono **sette** (schema config YAML, JSON output, semantica exit code, sort worst-first, nomi `check`/`target` che sono la **chiave di dedup** di `report-issues`/`alert`, formato JSONL di history/baseline, nomi e label delle metriche Prometheus) e nessuna ha un documento che dica se è stabile. Item pianificati (CF-153..160), deliberatamente **senza feature nuove**: **contratto di compatibilità** in `docs/compatibility.md` + campo di **versione dello schema** in JSON e history (senza il quale il primo cambio di formato rompe in silenzio `--diff` e `--fail-on-new` di chi ha già file su disco); **decisione scritta sulle chiavi ignote**, oggi scartate in silenzio da `check` perché il parse è volutamente tollerante (una config nuova deve girare su un binario vecchio) ma senza che nessuno lo sappia; **igiene di progetto** (`SECURITY.md` — la lacuna più vistosa per un tool che fa probe verso infrastruttura di terzi e legge token dall'ambiente —, `CONTRIBUTING.md`, template issue/PR); **split di `main.go`** (811 righe, il file che si tocca ogni volta che si aggiunge un flag, e i flag sono contratto); **copertura della superficie CLI** dal **17.3%** a ≥60% con test end-to-end sul binario sui percorsi contrattuali; **buchi di copertura** su `internal/moduledoc` (senza test, ed è la sorgente unica di `explain`/SARIF/desktop/skill) e sui moduli col driver sotto il 60%; **percorsi di distribuzione verificati** (il workflow `brew-test` ha run in coda da 13-21 ore, quindi in pratica non ha mai validato che `brew install` funzioni); e infine due **scope call** — se il desktop è dentro il 1.0 (non è firmato, il systray è deferito a Wails v3: l'opzione onesta è CLI a 1.0 e desktop beta con versione propria) e il chiarimento della licenza noncommercial — seguite da **`v1.0.0-rc.1`**, un ciclo di uso reale, e solo dopo `v1.0.0`. **Fuori scope dichiarato**: M30 (insight) e M34 (skill) non bloccano il 1.0, sono additivi e stanno in 1.1 — aggiungere feature prima del tag sposterebbe solo il traguardo. Nessun cambiamento al software: solo pianificazione; ogni item sarà una release a sé.

## 0.137.0

- CLI (CF-131 + CF-133, M31 — scaffold): `init` impara due modi di arrivare a una config utile senza partire dal foglio bianco. **`--recipe web|db|edge|media`** genera lo starter di uno stack invece di una lista di moduli: `web` = http+certs+dns, `db` = postgres+redis+tcp, `edge` = haproxy+certs+tcp+ntp, `media` = stream+ingest+http. Il valore non è risparmiare battute: è che chi inizia non sa ancora che un tier web ha bisogno di **`dns`** (i record derivano dopo una migrazione e nessun altro check se ne accorge) o che un edge ha bisogno di **`ntp`** (lo skew dell'orologio rompe TLS e i token molto prima che qualcuno sospetti l'orologio) — per questo ogni recipe porta con sé una riga che dice *perché* quei moduli. **`--from-inventory hosts.ini [--group web]`** genera i target dagli host di un inventory Ansible: l'inventory è già la fonte di verità, la config dovrebbe venire da lì e non da qualcuno che ribatte gli hostname. Gli indirizzi vengono da `ansible_host` dove c'è, altrimenti dal nome host; il file generato lo **dichiara in un commento**, insieme al fatto che per http/certs probabilmente si vuole il nome DNS pubblico (SNI e CN del certificato seguono il nome, non l'IP). Sono disponibili solo i moduli il cui target si ricava **dal solo hostname** (certs, tls, http, tcp, dns, ntp, redis, memcached, haproxy, consul, nats): chiedere `postgres` viene **rifiutato con una spiegazione** invece di inventare un DSN — una config che sembra pronta e fallisce alla prima run è peggio che sentirsi dire subito che non si può. Output **deterministico** (host ordinati per nome) così rigenerare dopo una modifica all'inventory produce il diff di quella modifica e nient'altro. Nessun segreto emesso: solo hostname e porte well-known, con le porte marcate come placeholder perché l'inventory sa gli host, non quale servizio intendevi. Validazione dei flag incompatibili (`--recipe` con `--from-inventory` o con `--modules`, `--group` senza inventory). TDD: `internal/scaffold/recipe_test.go` (ogni recipe nomina solo moduli scaffoldabili — un recipe rotto si scoprirebbe altrimenti nel momento peggiore, cioè quando un utente lo usa —, summary presenti, lookup case-insensitive) + `inventory_test.go` (tutti gli host usati, **determinismo su 5 run**, modulo non derivabile rifiutato con la lista di quelli validi, default certs+http, nessun segreto, ogni modulo dichiarato che rende davvero qualcosa per ogni host) + **`valid_test.go`, il test che conta**: ogni recipe, ogni snippet di modulo e ogni config da inventory passa `LoadConfig` + `Validate` **e** `Inspect` senza problemi bloccanti — quest'ultimo intercetterebbe uno scaffold che emette una chiave scritta male, che YAML scarterebbe in silenzio producendo una config che non controlla niente + `cmd/checkfleet/init_recipe_test.go` (le 4 recipe generate e validate end-to-end sul binario, from-inventory con `ansible_host` e `--group`, 7 combinazioni di flag errate, `--list`). Docs: sezioni "Recipes" e "From an Ansible inventory" in `docs/usage.md`.
- **M31 (Ergonomia & onboarding) chiusa**: `doctor` (CF-129), `targets` (CF-130), recipes (CF-131), validate con suggerimenti (CF-132) e scaffold da inventory (CF-133). Nota di processo: CF-131 e CF-133 sono rilasciati insieme invece che come due release separate — toccano le stesse due funzioni di `init` e di `scaffold`, e separarli avrebbe richiesto un commit intermedio artificiale che non compila, cosa peggiore della deviazione dalla regola "un item = una release".

## 0.136.0

- CLI (CF-132, M31 — validate con suggerimenti): `validate` non elenca più solo i problemi, **suggerisce il fix**. Il caso che conta di più è quello che prima non vedeva affatto: **le chiavi scritte male**. YAML **scarta in silenzio** quello che non riconosce, quindi `postgress:` invece di `postgres:` significa che il modulo **non gira mai** — nessun errore, nessun warning, e un `check all` che riporta una flotta sana perché non ha controllato niente; `validate` diceva solo "no module configured under `checks`" senza mai nominare il refuso. Ora confronta le chiavi del file con quelle che la config accetta davvero (lette dai tag yaml degli struct via reflection, così la lista non può divergere) sia al **top level** che sotto **`checks`**, e propone il match più vicino per **distanza di edit** (Levenshtein a due righe, zero-dep) o per **prefisso** (`elastic` → `elasticsearch`, che per Levenshtein è a 6 edit ma è quasi sempre l'intenzione). Se niente è abbastanza vicino **non suggerisce nulla**: un "did you mean" sbagliato con sicurezza è peggio di nessun suggerimento. Suggerimenti anche per i problemi già riportati: `checks` vuoto → il comando `init` da lanciare, modulo senza target → la chiave da riempire più `explain`, soglie invertite → che warn deve scattare prima di crit. Nuova distinzione **note vs problemi**: una `${VAR}` non settata riguarda **questa macchina**, non la config, quindi è una *nota* che **non fa fallire** `validate` — il comando è documentato per i pre-commit hook, e un laptop legittimamente non ha esportati i segreti di produzione: farlo fallire lì insegna solo a saltare l'hook. È `doctor` che tratta l'ambiente come soggetto e la considera BAD. Una config che **non carica** viene ora ispezionata comunque e riporta i problemi a livello di testo grezzo, che di solito ne sono la causa, invece di limitarsi all'errore del parser. Due falsi positivi trovati e corretti prima del rilascio: la chiave **`include`** (feature documentata CF-115) non è un campo di `Config` perché il loader la consuma dalla mappa raw, quindi una config corretta si sentiva dire "unknown key include"; e un `${...}` **dentro un commento** di `checkfleet.example.yml` veniva letto come una variabile chiamata `...` — ora un nome di env var deve avere la forma di un nome di env var. Nuovi `engine.Problem` (con `Advisory`), `engine.Inspect`, `engine.Blocking`; `Validate` resta invariata per i chiamanti esistenti. TDD: `internal/engine/suggest_test.go` (typo di modulo e top-level, **`include` come test di regressione**, nessun suggerimento per una chiave assurda, env advisory che non blocca, `Blocking`, placeholder nel commento, hint per `init`, config nil, `editDistance`, `closest` inclusi i prefissi, e un test che i set di chiavi derivino davvero dagli struct) + `cmd/checkfleet/validate_test.go` (typo nominato e suggerito, unset var che non fa fallire, `include` accettato, config non caricabile, config pulita senza note). Docs: sezione `validate` riscritta in `docs/usage.md` con il perché dei typo e la distinzione note/problemi.

## 0.135.0

- CLI (CF-129, M31 — preflight): nuovo comando `checkfleet doctor`, il "perché non funziona" in un comando — riporta sull'**ambiente** invece che sui servizi, con lo stesso vocabolario dei check (`engine.Finding` con OK/WARN/BAD/ERROR, quindi i renderer esistenti funzionano senza modifiche) in text o json. Quattro famiglie: **`env`** — le `${VAR}` referenziate dalla config e **non settate** (col nome esatto), quelle coperte solo da un `:-default` (WARN) e i `${file:…}` illeggibili; **`config`** — tutto quello che riporta `validate`, più il caso in cui la config **non carica**; **`target`** — indirizzi da cui non si ricava un host, porte implausibili, target duplicati; **`network`** — per host: risolve? la porta accetta una connessione TCP? Tre decisioni: (1) una `${VAR}` non settata è **BAD, non WARN** — si espande in **stringa vuota, in silenzio**, quindi la config parsa, il check gira e fallisce contro una password vuota con un errore che incolpa il database: nominare la variabile è tutto il motivo per cui questo comando esiste; (2) **funziona su una config che non carica** — la scansione delle variabili legge il file grezzo prima di qualsiasi parsing e il fallimento di load diventa un finding invece di un abort, perché una config rotta è esattamente quando serve un diagnostico; (3) i problemi di rete sono **ERROR, non BAD**, la stessa distinzione dei moduli ("non siamo riusciti a misurare da qui" ≠ "il target è malato"). Le probe sono deduplicate per `host:port` (40 URL sullo stesso host = una riga), concorrenti con semaforo, e su un IP literal la risoluzione viene saltata invece di riportare un errore DNS per qualcosa che non è mai stato un nome. Il comando **non gata**: esce 0 qualunque cosa trovi (regola M31). Nuovi: `engine.ScanVars`/`engine.VarRef` (scansione delle `${...}` sul testo grezzo, segue la catena degli `include`, rispetta l'escape `$${`, dedup per file, e non si ferma su una config illeggibile); `internal/doctor`; `coverage.Target.Port` — la porta è metadato non segreto e serve per fare la probe, mentre l'indirizzo può essere un DSN che non va mai stampato: senza, il target postgres del config d'esempio veniva riportato come "nessuna porta" pur avendo `:5432` nel DSN. Estratta `engine.SortFindings` dall'interno di `RunJobs` invece di duplicare un ordinamento che la doc dichiara "API di fatto". TDD: `internal/doctor/doctor_test.go` (mappatura dei quattro stati env, config valida/rotta, duplicati e porte, `portOf` con porte well-known per schema, **probe contro un listener locale su e una porta chiusa**, host irrisolvibile via `.invalid` RFC 2606, dedup 40→1, IP literal) + `internal/engine/vars_test.go` (i cinque tipi di riferimento, escape `$${`, dedup, catena di include con il file di provenienza, config non caricabile, ciclo di include che non deve appendere) + `cmd/checkfleet/doctor_test.go` (variabile non settata nominata, **config non parsabile con la variabile che emerge comunque**, probe con ERROR e `--no-probe` che non fa rete, json con `worst`, 2 errori sistemici). Nessuna rete esterna nei test. Docs: sezione "The `doctor` command" in `docs/usage.md` + README.

## 0.134.0

- CLI (CF-130, M31 — coverage): nuovo comando `checkfleet targets` che appiattisce **ogni target di ogni modulo** in una lista, in text o json, con `--module` per restringere. Risponde alla domanda che un config da 600 righe non risponde: *"il database nuovo l'ha aggiunto qualcuno al monitoring?"*. Con `--against hosts.ini` fa il **diff contro un inventory Ansible** (`--group web` per restringere il lato inventory): host coperti, **non monitorati**, e target che nell'inventory non ci sono. Quest'ultima sezione non è un errore — le dipendenze esterne legittimamente non stanno nell'inventory — ma va mostrata perché **un refuso in un target ha esattamente lo stesso aspetto**: un host che credevi di guardare e che non guarda niente. Match per hostname, case-insensitive, su **nome e `ansible_host`** dell'entry. Nuovo package `internal/coverage` con enumerazione **generica** (reflection su `ChecksConfig`) invece di un case scritto a mano per modulo: la lista a mano è il modo in cui una coverage tool finisce per **under-reportare in silenzio** — qualcuno aggiunge un modulo, si dimentica di aggiungerlo lì, e il tool risponde "sì, tutto coperto" perché non ha guardato. Il guard è `TestEveryModuleYieldsTargets`, che configura **tutti e 29** i moduli e pretende che ognuno produca almeno un target **con host**; ha già fatto il suo lavoro trovando `dns`, il cui `DNSTarget` non ha campo indirizzo perché il `Name` *è* il dominio da risolvere. **Niente segreti**: i target di postgres/mysql/mongodb sono DSN con la password dentro, e questa lista finisce in un terminale, in un file JSON e in un log di CI — viene estratto e stampato **solo l'hostname**, mai il DSN. L'estrazione gestisce le forme reali del formato (URL, `host:port`, DSN libpq `host=…`, DSN Go MySQL `user:pass@tcp(host:port)/`, URI di **replica set** con più membri: un solo target copre tutti e tre, e contarne uno solo sarebbe di nuovo under-reporting). Le ultime due le ha trovate il config d'esempio, non i test: la prima implementazione restituiva `mongo-01:27017,mongo-02:27017,mongo-03` come singolo host e **niente** per il DSN MySQL. Il comando **non gata**: esce 0 anche con host scoperti (regola M31 per i comandi diagnostici) — una lacuna di coverage è una decisione umana, non una build rossa. TDD: `internal/coverage/coverage_test.go` (matrice di estrazione host con le due forme sbagliate come casi di regressione, tre test anti-leak di credenziali di cui uno end-to-end sulla config, guard sui 29 moduli, diff con match via `ansible_host`, multi-host, case-insensitive, casi vuoti) + `cmd/checkfleet/targets_test.go` (render puro, sezioni di coverage, text+json sul binario, `--group`, exit 0 con host scoperti, 5 errori sistemici). Docs: sezione "The `targets` command" in `docs/usage.md` + README; corretta anche la tabella degli exit code, rimasta a `--exit-on-bad` dopo CF-134.

## 0.133.2

- Docs (backlog, planning): aggiunta la milestone **M34 — Integrazione con gli agenti (Claude Code skill) (fase 4)**. checkfleet è il tipo di tool che un agente dovrebbe saper invocare (config dichiarativa, `--output json` con `worst`, exit code con semantica precisa), ma oggi chi lo incontra **inventa le chiavi di config** e **sbaglia il gating**: la parte non ovvia non sta nell'`--help` — sta nel catalogo dei moduli (quale usare per quale sintomo), nello schema tipato, e nelle due regole controintuitive (**exit 0 anche con finding BAD**, **ERROR ≠ BAD**). Item pianificati (CF-149..152): **skill `checkfleet`** con sorgente in `skills/checkfleet/SKILL.md`, installata globalmente (non repo-local: il tool si usa dagli altri repo e dagli host); **reference generati** per moduli e schema di config (da `internal/moduledoc` — già sorgente unica di `explain`/SARIF/desktop — e per reflection su `engine.Config`, perché una copia a mano diverge alla prima release); **`checkfleet skill install|print`** con la skill embeddata nel binario (`go:embed`), così chi ha il binario ha la skill alla versione giusta; **gate CI anti-divergenza** (rigenera e fallisce sul diff) più `docs/agents.md`. Zero-dep, niente segreti negli esempi. **Fuori scope**: un server MCP — processo in ascolto e superficie di sicurezza propria, per un valore incrementale piccolo rispetto alla skill. Nessun cambiamento al software: solo pianificazione; ogni item sarà una release a sé.

## 0.133.1

- Repo metadata (CF-146, M33): description e homepage del repo aggiornate, e i topic rifatti. Prima erano quattro sinonimi tra loro — `monitor`, `monitoring`, `monitoring-automation`, `monitoring-tool` — che non intercettano nessuna ricerca reale: chi cerca su GitHub scrive `kafka`, `ansible`, `tls-certificates`, non "monitoring-tool". Ora sono **19 topic con le tecnologie effettivamente coperte** (`go`, `golang`, `cli`, `devops`, `sre`, `observability`, `health-check`, `healthcheck`, `infrastructure-monitoring`, `certificate-monitoring`, `tls-certificates`, `ansible`, `prometheus`, `nats`, `kafka`, `postgresql`, `consul`, `haproxy`, più `monitoring`), sotto il cap di 20 imposto da GitHub con uno slot di margine. Contano due volte: per la ricerca di GitHub e perché sono metadati che i crawler e i modelli leggono. **Non automatizzabile**: l'upload della social preview del repo resta da fare dalla UI (Settings → General), GitHub non espone API per quel campo — l'anteprima del *sito* docs è invece già coperta dai meta OpenGraph di 0.133.0.

## 0.133.0

- Docs / discoverability (CF-139…CF-145, M33 — SEO, answer engine & social): il prodotto era molto più avanti della sua distribuzione (29 moduli, desktop, GitHub Action… e 2 star), e il sito docs perdeva traffico su problemi banali. **Corretti**: `<title>` **duplicato** su ogni pagina (i layout ne emettevano uno e `jekyll-seo-tag` un altro — ora il plugin è chiamato con `title=false` e il layout resta padrone del titolo); **meta description identica su tutte le pagine** (ereditavano quella del sito: ora ognuna ha la sua, più `keywords` sulle pagine ad alto intento); `site.description` era anche il titolo della home, quindi è stata separata una `tagline` breve dalla descrizione lunga. **Aggiunti**: `sitemap.xml` e `robots.txt` come template Jekyll — niente `jekyll-sitemap`, il tema è in-repo e il Gemfile resta com'è; `og-card.png` 1200×630 (`docs/assets/`, sorgente `scripts/og-card.html` + `scripts/render-og-card.sh` per rigenerarla) iniettata via `defaults` perché `jekyll-seo-tag` legge `image` dal **front matter della pagina**, non da `site.image` — ed è anche ciò che promuove la Twitter card a `summary_large_image`; rimosso il blocco `twitter:` dal config, che senza un handle reale emetteva `twitter:site` uguale a `@`. **JSON-LD** in `docs/_includes/schema.html`: `SoftwareApplication` + `SoftwareSourceCode` + `WebSite` + `Person` in home, `TechArticle` + `BreadcrumbList` sulle pagine doc, `FAQPage` dove la pagina dichiara `faq:`. **Per gli answer engine** (la parte che conta di più adesso): `/llms.txt` — indice strutturato con i fatti, la semantica di status/exit code, comandi e moduli — e `/llms-full.txt`, tutta la documentazione in chiaro per l'ingestione in una fetch sola; `robots.txt` elenca esplicitamente GPTBot, ClaudeBot, PerplexityBot & co. come *Allow* (essere citati bene è l'obiettivo, non un rischio). **Due pagine nuove**, che sono le superfici su cui si atterra da una ricerca e che un assistente cita: `faq.md` (14 Q&A) e `comparison.md` (vs Prometheus, Blackbox exporter, Nagios/Icinga, Zabbix/Datadog, Uptime Kuma, bash a mano) con una sezione esplicita **"when *not* to use checkfleet"** — dire dove il tool non serve è ciò che rende credibile il resto, e la pagina ribadisce che checkfleet **non** sostituisce Prometheus. Nella FAQ le risposte vivono **una volta sola** nel front matter e generano sia il testo visibile sia il JSON-LD: non possono divergere, ed è esattamente ciò che Google richiede per i rich result. Nuovo `docs/_data/modules.yml` come **indice unico dei moduli**, da cui vengono la tabella riassuntiva di `modules.md`, il `featureList` dello schema e la sezione moduli di `llms.txt` — l'intro di `modules.md` era ferma a 18 moduli su 29, e con una fonte sola non può più succedere; gli heading dei moduli hanno ora anchor espliciti, così i link profondi non si rompono quando il titolo cambia. README: apertura riscritta come **risposta diretta** ("checkfleet is a command-line tool that…" — è il testo che finisce nello snippet di ricerca e nei riassunti degli assistenti) e sezione ripiegabile con il confronto e il link alle due pagine nuove. Nota sul limite: `robots.txt` resta **advisory** finché il sito vive sotto `/checkfleet` — i crawler lo leggono solo dalla root dell'host — mentre la sitemap è sottomettibile a Search Console comunque. Nessuna dipendenza nuova, nessun codice Go toccato; build del sito verificata in locale (Jekyll 4.3, JSON-LD parsato e validato, un solo `<title>` per pagina).
- Marketing: nuovo `marketing/SOCIAL-PLAN.md` (fuori dal sito) — posizionamento e messaggio, audience ordinate per fit, asset da preparare, sequenza di lancio a date, copy pronto per Show HN / r/devops / r/golang / Lobsters, cadenza settimanale sostenibile, metadata del repo, cosa misurare (incluso *cosa rispondono davvero* ChatGPT/Claude/Perplexity su "what is checkfleet") e i rischi — a partire dalla licenza PolyForm Noncommercial, che su HN va dichiarata per prima e con parole proprie.

## 0.132.2

- Fix (CI): la pipeline era **rossa da 20 run consecutive**, dal commit di CF-117 (0.125.0). Causa: l'overlay generico degli stack usa `reflect.Ptr`, alias **deprecato** di `reflect.Pointer` dal Go 1.18, e l'analizzatore `inline` di `govet` — arrivato con un aggiornamento della toolchain, che la CI segue via `go-version: stable` — ora lo segnala; con `golangci-lint` come hard gate, un finding = build rossa. Una riga in `internal/engine/config.go`: `reflect.Ptr` → `reflect.Pointer` (identici, `Ptr` è solo il vecchio nome). Nessun cambiamento di comportamento. **Perché non se n'era accorto nessuno**: `CLAUDE.md` definiva il gate come "`go vet` + `go test` (stessi check della CI)" ma la CI esegue **anche** `golangci-lint`, che `go vet` da solo non copre — corretto, ora il gate documentato include il linter e la versione da installare (v2.12.2, la stessa pinnata in CI; un binario v1 non legge il config schema v2). Aggiunta anche l'esclusione di `node_modules` in `.golangci.yml`: contiene un sorgente Go d'esempio dentro un pacchetto JS (flatted) che in CI non esiste ma in locale produce 2 finding non risolvibili — ed è così che quelli veri passano inosservati.

## 0.132.1

- Docs (CF-138): l'esempio dell'action in `docs/ci.md` puntava a `Allan-Nava/checkfleet@v1`, un tag **che non esiste** (i tag pubblicati sono `v0.x`) — un copia-incolla sarebbe fallito con "unable to resolve action". Ora punta al tag reale e chiarisce la distinzione tra il **tag a cui pinni l'action** e l'input `version`, che sceglie il binario checkfleet installato (default `latest`) ed è indipendente. Annotata anche la limitazione a runner Linux/macOS.

## 0.132.0

- CI (CF-138, M32 — GitHub Action riutilizzabile): nuovo `action.yml` **composite** alla radice, così integrare checkfleet passa da "scarica il binario e scrivi 20 righe di YAML" a `- uses: Allan-Nava/checkfleet@v1`. **Tutti gli input sono opzionali** e i default sono il caso comune (moduli `all` da `checkfleet.yml`, annotation + job summary, gate su BAD/ERROR): un job senza blocco `with:` deve già funzionare. Input: `version` (default `latest`), `module`, `config`, `stack`, `output`, `out-file`, `exit-on`, `exit-code`, `baseline`, `fail-on-new`, `min-severity`, `target`; output `exit-code` per reagire in workflow con `continue-on-error` senza rieseguire i check. Installa il **tarball di release** per l'arch del runner (linux/darwin × amd64/arm64, con errore esplicito su Windows e su arch ignote invece di un fallimento oscuro) e risolve `latest` tramite il **redirect di `/releases/latest`** anziché l'API: niente token e nessun rate limit non autenticato condiviso da sforare. **Sicurezza**: gli input arrivano a bash via `env:`, **mai** interpolati come `${{ }}` dentro il corpo dello script — un input con metacaratteri di shell verrebbe eseguito come codice. Distingue nel log l'exit 1 (checkfleet non è riuscito a partire) dal gate scattato. Aggiunto snippet **GitLab CI** completo in `docs/ci.md` (tarball + sink `junit` nel tab Tests, con `artifacts: when: always` — senza, un gate che fallisce butta via proprio il report che spiega perché). TDD `cmd/checkfleet/action_test.go`: YAML ben formato e `runs.using: composite`, ogni step con `shell`, ogni input con descrizione+default e **non** `required`, ogni input dichiarato **davvero referenziato** (un input dichiarato e ignorato è invisibile a chi lo setta), nessuna interpolazione nei corpi degli script, output che puntano a step id esistenti, e i **flag passati dall'action verificati contro la CLI reale** eseguendo il binario — è la giuntura che si rompe in silenzio, rinomini un flag e se ne accorge solo la pipeline di qualcun altro. Verifica funzionale: script dello step estratto ed eseguito su tre scenari (run verde, finding BAD con `exit-code` custom, config assente).
- **M32 (CI & pipelines) chiusa**: gate configurabile (CF-134), annotation GitHub + job summary (CF-135), SARIF per il Code scanning (CF-136), baseline / fail-on-new (CF-137) e action composite + GitLab (CF-138) — checkfleet è ora cittadino di prima classe di una pipeline, non un binario che qualcuno incolla in uno step.

## 0.131.0

- CLI (CF-137, M32 — baseline / fail-on-new): nuovi `--baseline FILE`, `--fail-on-new` e `--write-baseline` per adottare checkfleet su una flotta **già sporca**. Il problema che risolvono: con un gate normale la prima run è rossa e resta rossa, così il gate viene disattivato e smette di proteggere qualsiasi cosa. Il baseline **congela il debito noto** e fa fallire la build solo su ciò che è comparso — o peggiorato — da allora. Flusso: la prima run (file assente) **registra e non gata**, le successive confrontano. Gata su *nuovo* (target mai visto, o che era OK) e su *peggiorato* (WARN → BAD: una regressione su un target già imperfetto resta una regressione); **non** gata su debito invariato (BAD → BAD), miglioramenti (BAD → WARN) e finding spariti. I finding OK **vengono registrati**: senza, un target che passa da OK a BAD sarebbe indistinguibile da uno mai visto. Nuovo package `internal/baseline` (formato JSON versionato) che riusa `engine.DiffStatus` per la classificazione e la stessa chiave `check\ttarget` della vista `--diff`, così le due feature concordano su cosa sia "lo stesso finding". Tre scelte difensive: `--fail-on-new` **implica `--exit-on bad`** (senza soglia il flag sembrerebbe funzionare senza mai fallire); `--baseline` da solo **non tocca il gate** — restringerlo richiede `--fail-on-new`, così aggiungere un baseline a una pipeline non può disattivarne silenziosamente la protezione; un baseline illeggibile o di versione futura è un **errore sistemico (exit 1)**, non un baseline vuoto che lascerebbe passare tutto. TDD: `internal/baseline/baseline_test.go` (round-trip, versione ignota e file corrotto rifiutati, matrice a 8 casi di NewOrWorse inclusa la chiave che distingue i moduli, baseline vuoto, finding risolti) + 6 test d'integrazione sul binario in `cmd/checkfleet/baseline_test.go` (flusso di adozione completo con la **controprova** che senza baseline la stessa run è rossa, nuovo target che fallisce, `--write-baseline`, baseline che non allenta il gate, validazione dei flag, file corrotto) + `withImpliedThreshold` in `gate_test.go`. Docs: sezione "Adopting on a fleet that is already broken" in `docs/ci.md` con la tabella baseline→ora.

## 0.130.0

- Output (CF-136, M32 — SARIF): nuovo sink `--output sarif`, **SARIF 2.1.0** scritto a mano (zero-dep, `encoding/json`), così i finding finiscono nel tab **Code scanning / Security** di GitHub — con storico, dismissal e assegnazione — e in qualunque altro tool SARIF-aware. Ogni **modulo** è una *rule* (`checkfleet/certs`, …) con descrizione presa da `internal/moduledoc` (la stessa di `checkfleet explain`, nessuna duplicazione), ogni **finding** è un *result*. Livelli: BAD/ERROR → `error`, WARN → `warning`, OK → `none` (inclusi, non scartati: `none` è esattamente "esaminato, nulla da segnalare" e GitHub non ne fa alert). Tre scelte non ovvie, tutte per lo scarto tra un formato **orientato ai file** e finding che parlano di **target di rete**: (1) BAD ed ERROR condividono `error` perché SARIF non ha un terzo livello di fallimento, quindi lo status del motore resta recuperabile in `properties.status` — altrimenti "target malato" e "il check non è riuscito a misurare" diventerebbero indistinguibili; (2) i result sono **ancorati al file di config** (riga 1): GitHub scarta i result senza location, e il config è l'unico file del repo davvero responsabile del fatto che quel target venga controllato — il soggetto vero sta nel messaggio e in `properties.target` (usare un `--config` **relativo al repo** perché l'alert si agganci al file); (3) i `partialFingerprints` sono costruiti su check+target **escludendo la severità**, così un certificato che passa da WARN a BAD resta *lo stesso alert che peggiora* invece di aprirne uno nuovo. Portate anche le label globali (`properties["label.env"]`) e `value`/`unit`. TDD `internal/output/sarif_test.go` (envelope e `$schema`, una rule per modulo con `ruleIndex` coerente col `ruleId`, mappatura dei livelli, status preservato in properties, location/region + fallback del config path, fingerprint stabile al variare della severità, run vuota che serializza `results: []` e non `null` — un array nullo è SARIF invalido). Docs: sezione `sarif` in `docs/output.md` + "Code scanning (SARIF)" in `docs/ci.md` con il job completo e il permesso `security-events: write`.

## 0.129.0

- Output (CF-135, M32 — GitHub Actions): nuovo sink `--output github`. Emette i finding come **workflow command** (`::error` per BAD/ERROR, `::warning` per WARN) così **annotano inline** la run e la PR nella UI di GitHub, **e** scrive il report Markdown completo sul **job summary** (`$GITHUB_STEP_SUMMARY`) — in **append**, perché quel file è condiviso con gli altri step del job (un `atomicWrite` con rename butterebbe via il loro contenuto). I finding OK sono **volutamente omessi** dalle annotation: GitHub ne mostra al massimo 10 per livello per step, annotare i target verdi spingerebbe fuori dalla UI quelli veri; il summary invece li elenca tutti. Escaping conforme alla spec dei workflow command, con la distinzione che conta: nei **valori di property** vanno escapati anche `:` e `,` (quasi ogni target checkfleet contiene i due punti — schema URL, `host:port` — e uno non escapato **tronca l'annotation**), nel **messaggio** no; il `%` è sempre sostituito per primo, altrimenti corromperebbe gli escape inseriti dagli altri. **Perché il sink scrive il file da sé invece di documentare una pipe**: `... --exit-on bad >> "$GITHUB_STEP_SUMMARY" | tee` è **silenziosamente rotto**, la shell riporta l'exit code dell'ultimo comando della pipeline e quello di checkfleet viene sostituito dallo `0` di `tee` — il gate non scatta mai e il job resta verde qualunque cosa sia stato trovato (serve `set -o pipefail`, che la shell di default di `run:` **non** attiva). Senza pipe non c'è niente da sbagliare. Compone col fan-out (`--output github,slack`) e fuori da Actions stampa solo le annotation senza fallire. Zero-dep (solo formato). TDD: `internal/output/github_test.go` (mappatura livelli, OK saltati, matrice di escaping data-vs-property incluso il trap dell'ordine `%`, summary == markdown) + 4 test d'integrazione sul binario in `cmd/checkfleet/github_test.go` (annotation + summary, **append** su un summary preesistente, fuori da Actions, gate che scatta col summary comunque scritto). Docs: sezione `github` in `docs/output.md` e riscrittura dell'esempio GitHub Actions in `docs/ci.md` (da due run a una) con la spiegazione della trappola pipefail.

## 0.128.0

- CLI (CF-134, M32 — gate configurabile): `--exit-on warn|bad|error` sceglie la **soglia di severità** che rompe la build, `--exit-code N` (1-125) il **codice d'uscita**. Prima l'unico gate era `--exit-on-bad` (sempre exit 2 su BAD/ERROR): ora si può fallire già sul primo WARN, oppure **solo su ERROR** — cioè "i check non sono riusciti a misurare", ignorando i target malati che hanno un loro alerting. `--exit-on-bad` resta come **alias** di `--exit-on bad` (back-compat: default invariato, nessun gate se non lo chiedi, exit 2 quando scatta) e in caso di conflitto vince il flag esplicito. Due input rifiutati di proposito: `--exit-on ok` (farebbe fallire **ogni** run, anche tutta verde) e `--exit-code` fuori da 1-125 (`0` renderebbe il gate un no-op silenzioso, 126+ collide con l'intervallo della shell). I flag sono validati **prima** del run, così un refuso costa un errore di usage e non una sweep completa della flotta. Logica pura e isolata in `cmd/checkfleet/gate.go` (`parseGate` + `gate.exitCode`). Nuova `engine.AtLeast` per il confronto di severità esportato; `engine.ParseStatus` ora è **davvero** case-insensitive come già dichiarava il suo commento (prima accettava solo `warn`/`WARN`, non `Warn`). TDD `cmd/checkfleet/gate_test.go` (matrice soglia×worst per tutte e 4 le severità, alias legacy, precedenza del flag esplicito, codice custom, i tre input rifiutati) + verifica funzionale sul binario. Docs: sezione "Gate with `--exit-on`" riscritta in `docs/ci.md` con la tabella dei codici d'uscita e la distinzione **gate scattato (2) vs errore sistemico (1)** — confonderli significa, prima o poi, leggere "flotta sana" perché mancava il file di config.

## 0.127.0

- Engine + output (CF-119, M29 — label globali): nuovo `labels: {env: prod, region: eu}` in config, metadati operativi che il run porta con sé (`engine.Result.Labels`) e che compaiono negli output per routing e dashboard: su **ogni serie Prometheus** (`checkfleet_finding_status{check="…",target="…",env="prod",region="eu"}`, con il nome-label sanitizzato a `[a-zA-Z_][a-zA-Z0-9_]*`), come oggetto top-level `"labels"` in **JSON**, come **resource attributes** in **OTLP**, e come `.Labels` nei **template** webhook (`{{ .Labels.env }}`). Rispettate anche dal desktop. Zero-dep, **nessun segreto** (solo metadati), gli altri formati (text/markdown/…) le ignorano. **Back-compat**: senza `labels` l'output è byte-identico a prima. TDD `internal/output/labels_test.go` (Prometheus con label + sanitizzazione del nome, caso senza label, JSON, OTLP, template) + smoke. Docs: sezione "Global labels" in `docs/configuration.md` + `checkfleet.example.yml`.
- **M29 (Engine & scale) chiusa**: config include/`conf.d` (CF-115), cap di concorrenza globale (CF-116), composizione di stack (CF-117), fan-out multi-sink (CF-118) e label globali (CF-119) — checkfleet ora regge config estesi, molti target e più destinazioni.

## 0.126.0

- CLI (CF-118, M29 — fan-out multi-sink): `--output` accetta ora una **lista comma-separated**, così un singolo run emette verso **più sink insieme** senza rilanciare i check — es. `--output json,slack` stampa il JSON in locale **e** manda il report a Slack, oppure `--output markdown,slack,teams`. Con più sink ognuno è **isolato**: una env non settata o un webhook giù viene segnalato su stderr ma **non blocca gli altri sink né fa fallire la run** (il gate dei finding, `--exit-on-bad`, resta separato). Con `--output` singolo il comportamento è invariato (un errore del sink aborta come prima — back-compat). `--out-file` vale per il renderer di formato. TDD: unit `TestSplitCSV` + due test d'integrazione che eseguono il binario (`cmd/checkfleet/fanout_test.go`: il fan-out colpisce ogni sink verificato con un webhook `httptest`; un sink non configurato è isolato → exit 0 con il resto che parte e lo stderr che nomina il sink). Docs: nuova sezione in `docs/output.md`.

## 0.125.1

- Docs (backlog, planning): aggiunta la milestone **M32 — CI & pipelines (fase 4)**: rendere checkfleet cittadino di prima classe del CI, oltre l'attuale `--exit-on-bad`. Item pianificati (CF-134..138): **gate configurabile** (`--exit-on warn|bad|error` + `--exit-code N`), **output GitHub Actions annotations** (`--output github` con `::error::`/`::warning::` + job summary), **output SARIF** (per il tab Code scanning/Security), **baseline / fail-on-new** (gata solo sui finding nuovi vs una baseline, per adottare su flotte già "sporche"), **GitHub Action riutilizzabile** (`action.yml` composite + snippet GitLab CI — chiude M32). Tutto zero-dep, opt-in e retro-compatibile (l'exit-code di default non cambia). Nessun cambiamento al software — solo pianificazione; ogni item sarà una release a sé.

## 0.125.0

- Engine (CF-117, M29 — composizione di stack): `--stack` accetta ora una **lista comma-separated** applicata **in ordine, last-wins**, così i profili compongono — `--stack region-eu,prod` sovrappone `prod` su `region-eu` sulla base (base → regione → ambiente). Nuova `engine.LoadConfigStacks`; il caso singolo (`--stack prod`) resta identico (`LoadConfigStack` è ora un wrapper con un elemento). Ogni file di stack risolve i propri `include` (CF-115) prima dell'overlay. Vale su `check` e `serve` e nel desktop. **Fix latente**: l'overlay degli stack era una lista scritta a mano di 9 moduli e **ignorava silenziosamente** gli altri (redis, tls, tcp, kafka, …) — un modulo redis in uno stack non veniva applicato. Riscritto in modo **generico via reflection** sui campi di `Checks`, quindi ora **ogni modulo** (presente e futuro) è sovrascrivibile da uno stack; la semantica resta **wholesale-replace** (un modulo nello stack rimpiazza quello base, che riprende i suoi default) — invariata e coperta dal test esistente. TDD in-test (`stack_test.go`: composizione last-wins, override di un modulo prima ignorato come redis, entry vuote ignorate) + smoke funzionale (`--stack region,env` → target di env). Docs: sezione Multi-stack in `docs/configuration.md`.

## 0.124.0

- Engine (CF-116, M29 — cap di concorrenza globale): su flotte grandi un run non deve più aprire centinaia di connessioni tutte insieme. Nuovo `max_concurrency` in config + flag `--max-concurrency` (su `check` e `serve`, il flag vince) limitano quanti check girano contemporaneamente. Prima `engine.RunJobs` lanciava una goroutine per job **senza tetto**; ora `engine.RunJobsLimited`/`RunWithLimit` applicano un semaforo (canale bufferizzato) — **default `0` = illimitato**, quindi `RunJobs`/`RunWith` restano identici (back-compat) e chi non setta nulla non vede differenze. Il cap sta **sopra** gli eventuali semafori interni per-modulo (es. certs 16). Rispettato anche dal desktop (`RunChecks` e `WorkspaceStatus`). TDD in-test con un check strumentato che misura la **concorrenza di picco** (`internal/engine/concurrency_test.go`, 4 test: cap rispettato con 6 job/limite 2, illimitato = tutti insieme, cap>N non stalla, stesso risultato/ordine con e senza cap). Docs: `docs/configuration.md` (tabella top-level key) + `checkfleet.example.yml`.

## 0.123.0

- Engine (CF-115, M29 — config include / `conf.d`): una flotta grande ora si può **dividere in più file** per team/servizio e unirli in un solo config. `include: [path…]` accetta **file o directory** (relativi al file che include; una directory è un drop-in stile `conf.d/` che prende i suoi `*.yml`/`*.yaml` in ordine — prefissa `10-`, `20-` per controllarlo). Il merge è un **deep-merge** a livello di YAML map — robusto e future-proof, senza codice per-modulo: due file possono aggiungere moduli diversi sotto `checks:` e si combinano; ridefinire lo *stesso* modulo (o una lista come i suoi `targets:`) lo **rimpiazza**. L'**include-r vince** su ciò che include, tra include l'**ultimo vince**, gli include possono **annidarsi**; `${...}` è interpolato per-file; un **ciclo** o un file mancante è un errore di load chiaro. Compone con `--stack` (base e stack risolvono i propri include prima dell'overlay). Integrato in `parseConfig`, quindi vale ovunque si carichi un config. TDD in-test (`internal/engine/include_test.go`, 8 test). Docs: nuova sezione in `docs/configuration.md` + `include` nella tabella dei top-level key.

## 0.122.4

- Docs (backlog, planning): aggiunta la milestone **M31 — Ergonomia & onboarding (fase 4)**: rendere checkfleet più usabile e utile, dal primo avvio al valore in pochi minuti, con meno attrito nel diagnosticare e nel capire la copertura. Item pianificati (CF-129..133): **`checkfleet doctor`** (preflight “perché non funziona”: env non settate, target malformati, probe di raggiungibilità), **`checkfleet targets`** (coverage + diff contro inventory Ansible), **config recipes** (`init --recipe web|db|edge|media`), **validate con suggerimenti** (typo modulo → match più vicino, env mancante, fix azionabili), **scaffold da inventory** (`init --from-inventory hosts.ini` — chiude M31). Tutto zero-dep, riusa `internal/*`, testabile in-test. Nessun cambiamento al software — solo pianificazione; ogni item sarà una release a sé.

## 0.122.3

- Docs (backlog, planning): ampliata **M30 — Insight & intelligence** con altri quattro item (CF-125..128), sempre funzioni pure su history in `internal/insight`, zero-dep, riusati da CLI + desktop: **SLO error-budget & burn rate** (fast/slow burn stile SRE, “budget finito tra ~X”), **MTTR & durata outage corrente** (recovery time + “down da 47m, di solito ~8m”), **fleet health score** (indice 0–100 pesato per severità+stabilità, con trend), **“what changed” digest** (riassunto narrativo new/resolved/degrading/flapping, `--digest since=…` + drawer, inoltrabile ai sink M22 — chiude M30). Nessun cambiamento al software — solo pianificazione; ogni item sarà una release a sé.

## 0.122.2

- Docs (backlog, planning): aggiunta la milestone **M30 — Insight & intelligence (fase 4)**: trasformare la **history** che checkfleet già persiste (status + metrica + timestamp per target/run) in **segnale**, restando fedeli alla filosofia (analisi di dominio, non dashboard). Logica in un nuovo package **`internal/insight`** di funzioni pure, zero-dep (statistica a mano), riusato da CLI e desktop. Item pianificati (CF-120..124): **flapping detection** (target che oscillano), **trend forecast / ETA-to-threshold** (regressione lineare → “sfonda la soglia tra ~N giorni”), **anomaly / baseline deviation** (EWMA + z-score sulla norma recente), **correlation / blast-radius** (raggruppa i failure correlati per host/modulo/subnet), **runbooks & remediation hints** (URL/nota per-check mostrati negli output e nel drawer). Nessun cambiamento al software — solo pianificazione; ogni item sarà una release a sé.

## 0.122.1

- Docs (backlog, planning): aggiunta la milestone **M29 — Engine & scale (fase 4)**: rendere l'engine comodo su config estesi, molti target e più destinazioni, sempre zero-dep e senza segreti. Item pianificati (CF-115..119): **config include / `conf.d`** (split multi-file con deep-merge e cycle detection), **cap di concorrenza globale** (`max_concurrency`/`--max-concurrency`, oggi le goroutine sono illimitate), **composizione di stack** (`--stack a,b,c` last-wins), **fan-out multi-sink** (un run → più output insieme), **label globali** propagate agli output (Prometheus/JSON/OTLP/template). Nessun cambiamento al software — solo pianificazione; ogni item sarà una release a sé.

## 0.122.0

- Desktop (CF-114, M28 — action log / audit): ogni azione del workflow — un mute, un unmute, una nota, una issue aperta — lascia una riga nell'**Action log** (bottone **Actions** nella toolbar). È la timeline di *cosa abbiamo fatto davvero sulla flotta*, newest-first, con timestamp **UTC**, target e dettaglio. **Copy JSON** o **Copy Markdown** mettono l'intero log in clipboard (il Markdown è una tabella pronta da incollare in un handover o un postmortem) e **Clear** lo svuota. Il log è **locale** e bounded (ultime 200 azioni); come tutto l'incident workflow tiene solo storia operativa, mai segreti (una issue è loggata per URL, mai il token). Logica pura nel modulo UMD **`audit.js`** (add/sanitize/toJSON/toMarkdown, con escape di pipe/newline nel Markdown) con **TDD** `audit.test.js` (`node --test`, 5 test: prepend newest-first + kind vuoto ignorato, immutabilità + cap a MAX, sanitize, round-trip JSON, tabella Markdown con escaping). Docs (`docs/desktop.md`, `desktop/README.md`) aggiornate nello stesso commit con screenshot `docs/assets/desktop-actions.png`.
- **M28 (Desktop: dai finding alle azioni — incident workflow) chiusa**: da "vedo i problemi" a "li gestisco" — mute/snooze (CF-110), worst & monitor mute-aware (CF-111), note (CF-112), apri issue GitHub/GitLab (CF-113) e action log (CF-114). Tutto zero-dep, stato locale, nessun segreto nella UI.

## 0.121.0

- Desktop (CF-113, M28 — apri issue su GitHub/GitLab): il follow-up deferito da CF-106. Quando un finding **BAD/ERROR** è un problema vero che vuoi tracciare, dal suo drawer c'è **Report issue → GitHub / GitLab**: checkfleet apre una issue con titolo precompilato (`[checkfleet] check/target — STATUS`) e corpo (messaggio + contesto), poi mostra un toast con link **Open** per aprirla nel browser. Il pulsante compare **solo** per finding BAD/ERROR e **solo** per una forge **configurata** — repo/project e token arrivano **solo da variabili d'ambiente** (`GITHUB_REPO`/`GITHUB_TOKEN`, `GITLAB_PROJECT`/`GITLAB_TOKEN`, `*_API` opzionale per l'endpoint), mai dalla UI: nessun token tocca lo storage dell'app o un file di config. Client REST **zero-dep** (`issue.go`, HTTP/JSON a mano; GitHub via `Authorization: Bearer`, GitLab via `PRIVATE-TOKEN`) con binding `OpenIssue`/`IssueForges`/`OpenURL` e **TDD** via `httptest` (`issue_test.go`: endpoint/auth/payload GitHub, GitLab, non-configurato, forge detection). È un *“apri una issue per QUESTO finding”* one-click, distinto dal reconciler bulk `report-issues` del CLI (che pilota i CLI `gh`/`glab`). Docs (`docs/desktop.md`, `desktop/README.md`) aggiornate nello stesso commit con screenshot `docs/assets/desktop-issue.png`.

## 0.120.0

- Desktop (CF-112, M28 — note per finding): mutare dice *“ignora per ora”*, una **nota** dice *perché*. Dal detail drawer di un finding aggiungi un **owner** (opzionale) e una riga di contesto — *“Marco — ingest pool drenato per la migrazione edge, atteso fino alle 18:00”*. Il finding porta poi un chip **note** in tabella (hover per il testo) e la nota ritorna alla riapertura. Le note condividono l'identità del finding con i mute (**config + check + target**), quindi un target può essere sia mutato sia annotato; svuotando entrambi i campi la nota si cancella. Come i mute sono **contesto operativo locale** — mai scritto nello YAML, mai inviato altrove, nessun segreto. Logica nel modulo UMD **`notes.js`** (zero-dep, come `acks.js`/`presets.js`) con **TDD** `notes.test.js` (`node --test`, 5 test: normalize/set-upsert-e-delete/remove-get-has/describe/sanitize). Docs (`docs/desktop.md`, `desktop/README.md`) aggiornate nello stesso commit con screenshot `docs/assets/desktop-note.png`.

## 0.119.1

- Docs (backlog, riconciliazione): marcati come **chiusi** nel `BACKLOG.md` i 10 item di **M22 / M23 / M24** che erano già implementati, wired e testati nel codice ma con la checkbox rimasta aperta — nessun cambiamento al software, solo allineamento del backlog. Riepilogo con versione di rilascio: **CF-80** Telegram (v0.80.0), **CF-81** CSV (v0.81.0), **CF-82** webhook con template (v0.82.0), **CF-83** SNS/SigV4 (v0.91.0, chiude M22); **CF-84** override timeout/retry per-modulo (v0.98.0), **CF-85** maintenance ricorrenti daily/weekdays (v0.97.0), **CF-86** dedup + sort documentato (v0.96.0, chiude M23); **CF-87** self-metrics (v0.92.0), **CF-88** `/healthz`+`/readyz` (v0.93.0), **CF-89** logging JSON via `slog` (v0.95.0, chiude M24). Unico item ancora aperto fuori da M28: **CF-21** (modulo `mediamtx`), deprioritizzato.

## 0.119.0

- Desktop (CF-111, M28 — monitor & summary mute-aware): mutare un finding ora è più che cosmetico. Il **worst pill** del summary è calcolato sui finding **non mutati**, quindi una flotta i cui unici problemi rossi sono snoozati legge il suo livello successivo (es. **BAD** invece di ERROR) invece di gridare al peggio. I conteggi grezzi restano grezzi (vedi ancora *2 ERROR*) e la status bar tiene il tally onesto *“N muted”*: niente è nascosto, semplicemente smette di urlare. Anche il **monitor in background** usa lo stesso worst mute-aware — un finding snoozato non fa scattare la notifica nativa né alza il badge **● monitoring** finché il mute è valido. I mute *until recovery* si **auto-cancellano** appena il target torna verde. Il set di mute è spinto lato Go via binding `SetMutedKeys` (chiave `configPathchecktarget`, stesso `diffSep` del lato JS) così il monitor off-thread lo rispetta (`effectiveWorst`). TDD: `TestMonitorMuteAware` (effectiveWorst con/senza mute, sample all-muted → OK e nessun alert, unmute ripristina il problema). Docs (`docs/desktop.md`, `desktop/README.md`) aggiornate nello stesso commit con screenshot `docs/assets/desktop-mute-aware.png`.

## 0.118.0

- Desktop (CF-110, M28 — mute/snooze dei finding): alcuni problemi li conosci già (un cert che ruoti domani, un nodo spento apposta). Dal **detail drawer** di un finding ora puoi **silenziarlo** per **1h / 8h / 24h** o **fino al recovery**. Un finding mutato si **attenua** in tabella con un chip **muted**, la status bar mostra un conteggio **“N muted”**, e il toggle **Hide muted** nella toolbar li toglie di mezzo. I mute sono keyed per **config + check + target** (seguono il target esatto tra i run, non si mescolano tra flotte) e usano scadenze **assolute**, ripulite al load — un mute messo alle 18:00 per 1h è sparito alle 19:00 anche se riapri l'app nel mezzo; quelli *until recovery* restano finché non li togli. Tutto **locale** (localStorage): un mute è una nota operativa, mai una modifica allo YAML, e non contiene segreti. La logica vive nel modulo UMD **`acks.js`** (zero-dep, come `charts.js`/`presets.js`) con **TDD headless** `acks.test.js` (`node --test`, 8 test: key/durationUntil/isMuted/mute-unmute immutabili/prune/activeCount/describe/normalize). Docs (`docs/desktop.md`, `desktop/README.md`) aggiornate nello stesso commit con screenshot `docs/assets/desktop-mute.png`. Apre **M28 — dai finding alle azioni (incident workflow)**.

## 0.117.0

- Desktop (CF-109, M27 — monitoraggio in background): **Auto** ora avvia un monitor periodico **lato Go**, non un semplice timer nel browser. Un ticker in una goroutine riusa la stessa pipeline `RunChecks`, quindi ogni passata è un run reale dei moduli (trend e history continuano a riempirsi anche mentre sei su un'altra vista) e i risultati arrivano al frontend via evento Wails `monitor:sample`, renderizzati **off-thread**. Un badge **● monitoring** nella status bar segnala che è vivo, colorato per worst status. Il punto di un monitor è avvisarti quando qualcosa **cambia**, non tempestarti: la notifica nativa scatta **solo al cambio di worst status** (degraded / improved / recovered) — una flotta che resta BAD per un'ora notifica una volta, non sessanta. La logica di dedup è la funzione pura `monitorAlert`, con **TDD** (`monitor_test.go`: tabella alert esaustiva + baseline del sample sulla pipeline reale + lifecycle Start/Stop). Cambiare config/stack/interval ri-punta il monitor. In preview browser (senza backend Wails) resta il fallback a timer JS. Binding `StartMonitor`/`StopMonitor`/`MonitorRunning`. Docs (`docs/desktop.md`, `desktop/README.md`) aggiornate nello stesso commit con screenshot `docs/assets/desktop-monitor.png`.
- Nota (M27 chiusa): l'**icona colorata in menu-bar/tray** con voce "Open checkfleet" prevista in CF-109 è **deferita**. Wails v2 non ha un systray integrato (arriva in Wails v3) e agganciarlo con una libreria terza contende il main-thread Cocoa su macOS: è stato scelto di non introdurre una dipendenza fragile/non verificabile headless e rimandare il tray al passaggio a Wails v3. Con CF-104..109 la milestone **M27 (Desktop power & workflow)** è completa, tray a parte.

## 0.116.0

- Desktop (CF-108, M27 — viste salvate/preset): nuova barra **Views** sotto la toolbar che nomina e salva le lenti che riguardi spesso — *prod errors only*, *certs in scadenza*, *tutto raggruppato per modulo*. Una vista cattura l'intero stato della toolbar (**stack + filtro + min-severity + Group + vista aperta**) sotto un nome; i **chip** la riapplicano in un click e si **illuminano quando la toolbar già corrisponde**, la **✕** la cancella. **＋ Save view** salva lo stato corrente (riusare un nome sovrascrive). **Import/Export** spostano l'intero set come **JSON via clipboard** (nomi coincidenti sovrascritti) — comodo per condividere le lenti standard di un team o inizializzare una nuova macchina. Le viste sono presenti anche nella command palette (⌘K → *View: …*, *Save current view*). Sono **pura UI state** in localStorage: nessun contenuto di config, nessun segreto. La logica vive nel modulo UMD **`presets.js`** (zero-dep, stessa forma di `charts.js`) con **TDD headless**: `presets.test.js` (`node --test`) — normalize/upsert/remove/matches/serialize/parse, 10 test. Docs (`docs/desktop.md`, `desktop/README.md`) aggiornate nello stesso commit con screenshot `docs/assets/desktop-views.png`.

## 0.115.0

- Desktop (CF-107, M27 — workspace multi-config): nuovo **pannello Workspace** (pulsante a griglia nella title bar) che tiene insieme tutte le flotte usate sulla macchina — prod, staging, edge, lab, ognuna col suo `checkfleet.yml`. I config vengono **ricordati automaticamente** a ogni open/run (nessuno step "add" separato), MRU-first, fino a 20, e sopravvivono ai riavvii (localStorage: **solo i path**, mai contenuti o credenziali); si può anche fissarne uno con **+ Add config**. **Run all** valuta ogni config in modo indipendente (con il proprio stack), riempie i badge per-fleet con i conteggi `OK·WARN·BAD·ERROR` e fa il **rollup del worst aggregato** nel badge in testa al pannello — a colpo d'occhio si vede se *qualcosa, ovunque*, richiede attenzione. Click su una riga per switchare il fleet attivo (ricarica i suoi stack). Binding `App.WorkspaceStatus([]path) []ConfigStatus` (riusa `loadConfig`/`registry.Configured`/`engine.RunWith`/`Summarize`/`Worst`; config mancante → `ERROR` per quella riga, le altre proseguono). TDD `TestWorkspaceStatus`; smoke frontend verde; docs (`docs/desktop.md`, `desktop/README.md`) aggiornate nello stesso commit con screenshot `docs/assets/desktop-workspace.png`.

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
