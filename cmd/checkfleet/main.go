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
	"sync"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/checks/certs"
	"github.com/Allan-Nava/checkfleet/internal/checks/consul"
	"github.com/Allan-Nava/checkfleet/internal/checks/dns"
	"github.com/Allan-Nava/checkfleet/internal/checks/haproxy"
	"github.com/Allan-Nava/checkfleet/internal/checks/httpcheck"
	"github.com/Allan-Nava/checkfleet/internal/checks/nats"
	"github.com/Allan-Nava/checkfleet/internal/checks/patroni"
	"github.com/Allan-Nava/checkfleet/internal/checks/postgres"
	"github.com/Allan-Nava/checkfleet/internal/checks/stream"
	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/output"
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
  checkfleet check <all|certs|http|nats|haproxy|stream|patroni|consul|postgres|dns> --config checkfleet.yml [--output text|markdown|json|slack] [--exit-on-bad]
  checkfleet serve --config checkfleet.yml [--listen :9876] [--interval 60s]   # esporta le metriche Prometheus
  checkfleet report-issues --config checkfleet.yml [--dry-run]                 # apre/chiude issue GitHub dai finding BAD
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
	exitOnBad := fs.Bool("exit-on-bad", false, "exit code 2 se presenti finding BAD/ERROR")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	cfg, err := loadConfig(*configPath, *stack)
	if err != nil {
		return err
	}

	specs := modules(cfg)
	var selected []engine.Check
	known := module == "all"
	for _, s := range specs {
		if module != "all" && module != s.name {
			continue
		}
		known = true
		if !s.configured {
			if module == s.name {
				return fmt.Errorf("modulo %q non configurato in %s", s.name, *configPath)
			}
			continue
		}
		selected = append(selected, s.build())
	}
	if !known {
		return fmt.Errorf("modulo %q sconosciuto", module)
	}
	if len(selected) == 0 {
		return fmt.Errorf("nessun modulo selezionato (niente configurato per %q)", module)
	}

	res := engine.Run(context.Background(), selected, time.Duration(cfg.TimeoutSeconds)*time.Second)

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

// loadConfig loads the base config, overlaying a stack profile when set.
func loadConfig(path, stack string) (*engine.Config, error) {
	if stack != "" {
		return engine.LoadConfigStack(path, stack)
	}
	return engine.LoadConfig(path)
}

// moduleSpec ties a module name to whether it's configured and how to build it.
type moduleSpec struct {
	name       string
	configured bool
	build      func() engine.Check
}

// modules is the single registry of check modules, shared by `check` and
// `serve` so the wiring lives in one place.
func modules(cfg *engine.Config) []moduleSpec {
	c := cfg.Checks
	return []moduleSpec{
		{"certs", c.Certs != nil, func() engine.Check { return certs.New(*c.Certs) }},
		{"http", c.HTTP != nil, func() engine.Check { return httpcheck.New(*c.HTTP) }},
		{"nats", c.NATS != nil, func() engine.Check { return nats.New(*c.NATS) }},
		{"haproxy", c.HAProxy != nil, func() engine.Check { return haproxy.New(*c.HAProxy) }},
		{"stream", c.Stream != nil, func() engine.Check { return stream.New(*c.Stream) }},
		{"patroni", c.Patroni != nil, func() engine.Check { return patroni.New(*c.Patroni) }},
		{"consul", c.Consul != nil, func() engine.Check { return consul.New(*c.Consul) }},
		{"postgres", c.Postgres != nil, func() engine.Check { return postgres.New(*c.Postgres) }},
		{"dns", c.DNS != nil, func() engine.Check { return dns.New(*c.DNS) }},
	}
}

// configuredChecks builds every configured module (used by `serve`).
func configuredChecks(cfg *engine.Config) []engine.Check {
	var checks []engine.Check
	for _, s := range modules(cfg) {
		if s.configured {
			checks = append(checks, s.build())
		}
	}
	return checks
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
	checks := configuredChecks(cfg)
	if len(checks) == 0 {
		return fmt.Errorf("nessun modulo configurato in %s", *configPath)
	}
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second

	var mu sync.Mutex
	var latest engine.Result
	runOnce := func() {
		res := engine.Run(context.Background(), checks, timeout)
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
