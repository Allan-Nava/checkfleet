// checkfleet: a fleet of infrastructure checks with one binary.
//
//	checkfleet check all   --config checkfleet.yml [--output text|markdown|json]
//	checkfleet check certs --config checkfleet.yml
//	checkfleet version
//
// Exit code: 0 also on WARN/BAD findings (a check that ran IS a success —
// gate on the output instead, or pass --exit-on-bad to get exit 2 when any
// BAD/ERROR finding is present). Non-zero otherwise only for systemic errors
// (unreadable config, unknown module).
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/history"
	"github.com/Allan-Nava/checkfleet/internal/output"
	"github.com/Allan-Nava/checkfleet/internal/registry"
)

var version = "dev" // injected at build time via -ldflags

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(64)
	}
	switch os.Args[1] {
	case "version":
		fmt.Println("checkfleet", version)
	case "check":
		if err := runCheck(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "checkfleet:", err)
			os.Exit(1)
		}
	case "serve":
		if err := runServe(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "checkfleet:", err)
			os.Exit(1)
		}
	case "validate":
		if err := runValidate(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "checkfleet:", err)
			os.Exit(1)
		}
	case "report-issues":
		if err := runReportIssues(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "checkfleet:", err)
			os.Exit(1)
		}
	default:
		usage()
		os.Exit(64)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `uso:
  checkfleet check <all|certs|http|nats|haproxy|stream|patroni|consul|postgres|dns|redis|keycloak|tcp|tls|ntp|rabbitmq|grpc> --config checkfleet.yml [--output text|markdown|json|slack] [--only ...] [--min-severity warn] [--target glob] [--exit-on-bad]
  checkfleet serve --config checkfleet.yml [--listen :9876] [--interval 60s]   # esporta le metriche Prometheus
  checkfleet report-issues --config checkfleet.yml [--dry-run]                 # apre/chiude issue GitHub dai finding BAD
  checkfleet validate --config checkfleet.yml                                  # valida la config senza eseguire i check
  checkfleet version`)
}

func runCheck(args []string) error {
	if len(args) < 1 {
		usage()
		return fmt.Errorf("modulo mancante")
	}
	module := args[0]

	fs := flag.NewFlagSet("check", flag.ExitOnError)
	configPath := fs.String("config", "checkfleet.yml", "file di configurazione YAML")
	stack := fs.String("stack", "", "profilo stack: sovrappone checkfleet.<stack>.yml alla base")
	format := fs.String("output", "text", "formato: text, markdown, json, slack")
	webhookEnv := fs.String("webhook-env", "SLACK_WEBHOOK", "env var con l'URL webhook Slack (output slack)")
	only := fs.String("only", "", "mostra solo questi check (lista separata da virgole)")
	minSeverity := fs.String("min-severity", "", "mostra solo finding a partire da: ok|warn|bad|error")
	targetGlob := fs.String("target", "", "mostra solo i target che matchano questo glob")
	historyPath := fs.String("history", "", "file JSONL di storico: registra il run e segnala il flapping")
	flapChanges := fs.Int("flap-changes", 3, "n. minimo di cambi di stato per segnalare flapping")
	flapWindow := fs.Int("flap-window", 10, "n. di run recenti su cui valutare il flapping")
	exitOnBad := fs.Bool("exit-on-bad", false, "exit code 2 se presenti finding BAD/ERROR")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	minSev, ok := engine.ParseStatus(*minSeverity)
	if !ok {
		return fmt.Errorf("--min-severity %q non valido (usa ok|warn|bad|error)", *minSeverity)
	}
	filter := engine.FilterOptions{Only: commaSet(*only), MinSeverity: minSev, TargetGlob: *targetGlob}

	cfg, err := loadConfig(*configPath, *stack)
	if err != nil {
		return err
	}

	specs := registry.Modules(cfg)
	var selected []engine.Check
	known := module == "all"
	for _, s := range specs {
		if module != "all" && module != s.Name {
			continue
		}
		known = true
		if !s.Configured {
			if module == s.Name {
				return fmt.Errorf("modulo %q non configurato in %s", s.Name, *configPath)
			}
			continue
		}
		selected = append(selected, s.Build())
	}
	if !known {
		return fmt.Errorf("modulo %q sconosciuto", module)
	}
	if len(selected) == 0 {
		return fmt.Errorf("nessun modulo selezionato (niente configurato per %q)", module)
	}

	res := engine.RunWith(context.Background(), selected, runOptions(cfg))
	if *historyPath != "" {
		flaps, err := recordHistory(*historyPath, res, *flapChanges, *flapWindow)
		if err != nil {
			fmt.Fprintln(os.Stderr, "checkfleet: storico:", err)
		}
		res.Findings = append(res.Findings, flaps...)
	}
	res.Findings = engine.Filter(res.Findings, filter)

	switch *format {
	case "text":
		fmt.Print(output.Text(res))
	case "markdown":
		fmt.Print(output.Markdown(res, module))
	case "json":
		s, err := output.JSON(res)
		if err != nil {
			return err
		}
		fmt.Println(s)
	case "slack":
		payload, err := output.Slack(res, module)
		if err != nil {
			return err
		}
		webhook := os.Getenv(*webhookEnv)
		if webhook == "" {
			return fmt.Errorf("webhook Slack non impostato: la env %s è vuota", *webhookEnv)
		}
		if err := postSlack(context.Background(), webhook, payload); err != nil {
			return err
		}
		fmt.Println("checkfleet: report inviato a Slack")
	default:
		return fmt.Errorf("formato %q sconosciuto", *format)
	}

	if *exitOnBad {
		worst := engine.Worst(res.Findings)
		if worst == engine.BAD || worst == engine.ERROR {
			os.Exit(2)
		}
	}
	return nil
}

// runValidate checks the config without running any check; exit 1 if invalid.
//
//	checkfleet validate --config checkfleet.yml [--stack …]
func runValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	configPath := fs.String("config", "checkfleet.yml", "file di configurazione YAML")
	stack := fs.String("stack", "", "profilo stack: sovrappone checkfleet.<stack>.yml alla base")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := loadConfig(*configPath, *stack)
	if err != nil {
		return err // parse/read errors are already fatal
	}
	problems := engine.Validate(cfg)
	if len(problems) == 0 {
		fmt.Printf("checkfleet: %s valida ✅\n", *configPath)
		return nil
	}
	fmt.Fprintf(os.Stderr, "%s: %d problema/i:\n", *configPath, len(problems))
	for _, p := range problems {
		fmt.Fprintln(os.Stderr, "  -", p)
	}
	os.Exit(1)
	return nil
}

// recordHistory appends this run to the JSONL history and returns WARN
// findings for keys that are flapping across the recent window.
func recordHistory(path string, res engine.Result, minChanges, window int) ([]engine.Finding, error) {
	store := history.Open(path)
	rec := history.Record{Unix: res.Started.Unix()}
	for _, f := range res.Findings {
		rec.Entries = append(rec.Entries, history.Entry{Check: f.Check, Target: f.Target, Status: string(f.Status)})
	}
	if err := store.Append(rec); err != nil {
		return nil, err
	}
	recent, err := store.Recent(window)
	if err != nil {
		return nil, err
	}
	var flaps []engine.Finding
	for _, fl := range history.Flaps(recent, minChanges) {
		flaps = append(flaps, engine.Finding{
			Check: "flap", Target: fl.Key, Status: engine.WARN,
			Message: fmt.Sprintf("flapping: %d cambi di stato negli ultimi %d run (ora %s)", fl.Changes, len(recent), fl.Last),
		})
	}
	return flaps, nil
}

// commaSet parses a comma-separated flag into a set (nil when empty).
func commaSet(s string) map[string]bool {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	set := map[string]bool{}
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			set[p] = true
		}
	}
	return set
}

// runOptions builds the engine run options from the config.
func runOptions(cfg *engine.Config) engine.Options {
	return engine.Options{
		Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second,
		Retries: cfg.Retries,
		Backoff: time.Duration(cfg.RetryBackoffMS) * time.Millisecond,
	}
}

// loadConfig loads the base config, overlaying a stack profile when set.
func loadConfig(path, stack string) (*engine.Config, error) {
	if stack != "" {
		return engine.LoadConfigStack(path, stack)
	}
	return engine.LoadConfig(path)
}

// runServe exposes the findings as Prometheus metrics, re-running the checks on
// an interval. checkfleet serve --config … --listen :9876 --interval 60s
func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	configPath := fs.String("config", "checkfleet.yml", "file di configurazione YAML")
	stack := fs.String("stack", "", "profilo stack: sovrappone checkfleet.<stack>.yml alla base")
	listen := fs.String("listen", ":9876", "indirizzo di ascolto")
	interval := fs.Duration("interval", 60*time.Second, "intervallo di riesecuzione dei check")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := loadConfig(*configPath, *stack)
	if err != nil {
		return err
	}
	checks := registry.Configured(cfg)
	if len(checks) == 0 {
		return fmt.Errorf("nessun modulo configurato in %s", *configPath)
	}
	opts := runOptions(cfg)

	var mu sync.Mutex
	var latest engine.Result
	runOnce := func() {
		res := engine.RunWith(context.Background(), checks, opts)
		mu.Lock()
		latest = res
		mu.Unlock()
	}
	runOnce()
	go func() {
		t := time.NewTicker(*interval)
		defer t.Stop()
		for range t.C {
			runOnce()
		}
	}()

	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		res := latest
		mu.Unlock()
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		fmt.Fprint(w, output.Prometheus(res))
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "checkfleet %s\n\nmetriche: /metrics\n%d moduli, riesecuzione ogni %s\n", version, len(checks), *interval)
	})
	fmt.Fprintf(os.Stderr, "checkfleet serve: %d moduli su %s (intervallo %s)\n", len(checks), *listen, *interval)
	return http.ListenAndServe(*listen, nil)
}

// postSlack sends a Block Kit payload to an incoming webhook URL.
func postSlack(ctx context.Context, webhook, payload string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhook, bytes.NewBufferString(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("invio a Slack: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Slack ha risposto HTTP %d", resp.StatusCode)
	}
	return nil
}
