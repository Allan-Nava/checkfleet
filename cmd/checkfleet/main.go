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
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
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
	case "init":
		if err := runInit(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "checkfleet:", err)
			os.Exit(1)
		}
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
	case "alert":
		if err := runAlert(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "checkfleet:", err)
			os.Exit(1)
		}
	case "explain":
		if err := runExplain(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "checkfleet:", err)
			os.Exit(1)
		}
	case "completion":
		if err := runCompletion(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "checkfleet:", err)
			os.Exit(1)
		}
	default:
		usage()
		os.Exit(64)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  checkfleet init [--modules certs,http] [--config checkfleet.yml] [--force]     # scaffold a starter config
  checkfleet check <all|certs|http|nats|haproxy|stream|patroni|consul|postgres|dns|redis|keycloak|tcp|tls|ntp|rabbitmq|grpc|ldap|kafka|ingest|s3|smtp|elasticsearch|mongodb|mysql|etcd|clickhouse|vault|memcached|cassandra> --config checkfleet.yml [--output text|markdown|json|junit|html|prometheus|otlp|csv|slack|discord|teams|telegram|webhook,...] [--out-file PATH] [--no-color] [--only ...] [--min-severity warn] [--target glob] [--watch 5s] [--history F --diff] [--max-concurrency N] [--exit-on-bad]
  checkfleet serve --config checkfleet.yml [--listen :9876] [--interval 60s] [--max-concurrency N]   # export Prometheus metrics
  checkfleet report-issues --config checkfleet.yml [--forge github|gitlab]     # open/close tracker issues from BAD findings
  checkfleet alert --config checkfleet.yml --provider pagerduty --key-env K    # create/resolve on-call alerts from BAD/ERROR
  checkfleet validate --config checkfleet.yml                                  # validate the config without running the checks
  checkfleet explain [module]                                                 # what a module checks and its thresholds
  checkfleet completion <bash|zsh|fish>                                        # print a shell completion script
  checkfleet version`)
}

func runCheck(args []string) error {
	if len(args) < 1 {
		usage()
		return fmt.Errorf("missing module")
	}
	module := args[0]

	fs := flag.NewFlagSet("check", flag.ExitOnError)
	configPath := fs.String("config", "checkfleet.yml", "YAML config file")
	stack := fs.String("stack", "", "comma-separated stack profiles overlaid in order (last wins): checkfleet.<stack>.yml onto the base")
	format := fs.String("output", "text", "output sink(s), comma-separated to fan out (e.g. text,slack): text, markdown, json, junit, html, prometheus, otlp, csv, slack, discord, teams, telegram, webhook")
	outFile := fs.String("out-file", "", "write the output to this file (atomically) instead of stdout")
	noColor := fs.Bool("no-color", false, "disable ANSI colour in the text output (also honours NO_COLOR)")
	webhookEnv := fs.String("webhook-env", "SLACK_WEBHOOK", "env var holding the Slack webhook URL (slack output)")
	tgTokenEnv := fs.String("telegram-token-env", "TELEGRAM_TOKEN", "env var holding the Telegram bot token (telegram output)")
	tgChatEnv := fs.String("telegram-chat-env", "TELEGRAM_CHAT_ID", "env var holding the Telegram chat id (telegram output)")
	tmplFile := fs.String("template", "", "Go text/template file to shape the payload (webhook output)")
	only := fs.String("only", "", "show only these checks (comma-separated list)")
	minSeverity := fs.String("min-severity", "", "show only findings at or above: ok|warn|bad|error")
	targetGlob := fs.String("target", "", "show only targets matching this glob")
	historyPath := fs.String("history", "", "JSONL history file: record the run and flag flapping")
	flapChanges := fs.Int("flap-changes", 3, "minimum number of state changes to flag flapping")
	flapWindow := fs.Int("flap-window", 10, "number of recent runs to evaluate flapping over")
	pingURLEnv := fs.String("ping-url-env", "", "env var holding the dead-man's-switch URL (e.g. Healthchecks.io) to ping at the end of the run")
	watch := fs.Duration("watch", 0, "re-run on this interval with a live terminal view (e.g. 5s); Ctrl-C to stop")
	diff := fs.Bool("diff", false, "show only what changed vs the previous run (requires --history)")
	exitOnBad := fs.Bool("exit-on-bad", false, "exit code 2 if any BAD/ERROR finding is present")
	maxConc := fs.Int("max-concurrency", -1, "cap on checks running at once (0 = unbounded); overrides max_concurrency in the config")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	minSev, ok := engine.ParseStatus(*minSeverity)
	if !ok {
		return fmt.Errorf("--min-severity %q is not valid (use ok|warn|bad|error)", *minSeverity)
	}
	filter := engine.FilterOptions{Only: commaSet(*only), MinSeverity: minSev, TargetGlob: *targetGlob}

	cfg, err := loadConfig(*configPath, *stack)
	if err != nil {
		return err
	}

	base := runOptions(cfg)
	specs := registry.Modules(cfg)
	var selected []engine.Job
	known := module == "all"
	for _, s := range specs {
		if module != "all" && module != s.Name {
			continue
		}
		known = true
		if !s.Configured {
			if module == s.Name {
				return fmt.Errorf("module %q is not configured in %s", s.Name, *configPath)
			}
			continue
		}
		selected = append(selected, engine.Job{Check: s.Build(), Opts: registry.OptionsFor(cfg, s.Name, base)})
	}
	if !known {
		return fmt.Errorf("unknown module %q", module)
	}
	if len(selected) == 0 {
		return fmt.Errorf("no module selected (nothing configured for %q)", module)
	}

	limit := effectiveConcurrency(*maxConc, cfg)

	if *watch > 0 {
		watchColor := !*noColor && os.Getenv("NO_COLOR") == "" && isTerminal(os.Stdout)
		return runWatch(selected, cfg, filter, *watch, watchColor, limit)
	}

	res := engine.RunJobsLimited(context.Background(), selected, limit)
	res.Labels = cfg.Labels // global labels ride into the outputs (CF-119)
	if *historyPath != "" {
		flaps, err := recordHistory(*historyPath, res, *flapChanges, *flapWindow)
		if err != nil {
			fmt.Fprintln(os.Stderr, "checkfleet: history:", err)
		}
		res.Findings = append(res.Findings, flaps...)
	}
	res.Findings = engine.ApplyMaintenance(res.Findings, cfg.Maintenance, time.Now())
	res.Findings = engine.Filter(res.Findings, filter)

	if *diff {
		if *historyPath == "" {
			return fmt.Errorf("--diff requires --history")
		}
		recent, err := history.Open(*historyPath).Recent(2)
		if err != nil {
			return err
		}
		fmt.Print(formatDiff(diffFromRecords(recent)))
		if *exitOnBad {
			if w := engine.Worst(res.Findings); w == engine.BAD || w == engine.ERROR {
				os.Exit(2)
			}
		}
		return nil
	}

	// --output may be a comma-separated list, so one run fans out to several
	// sinks (e.g. text,slack). Color is only meaningful for a lone text sink on
	// a terminal.
	sinks := splitCSV(*format)
	if len(sinks) == 0 {
		sinks = []string{"text"}
	}
	color := len(sinks) == 1 && sinks[0] == "text" && *outFile == "" && !*noColor &&
		os.Getenv("NO_COLOR") == "" && isTerminal(os.Stdout)

	// emit sends the run to one sink: a push sink (slack/…) POSTs to its env URL,
	// a format renderer writes to --out-file or stdout.
	emit := func(sink string) error {
		switch sink {
		case "slack":
			payload, err := output.Slack(res, module)
			if err != nil {
				return err
			}
			url := os.Getenv(*webhookEnv)
			if url == "" {
				return fmt.Errorf("slack webhook not set: env %s is empty", *webhookEnv)
			}
			if err := postJSON(context.Background(), url, payload); err != nil {
				return err
			}
			fmt.Println("checkfleet: report sent to Slack")
		case "discord":
			return postRendered(*webhookEnv, "Discord", func() (string, error) { return output.Discord(res, module) })
		case "teams":
			return postRendered(*webhookEnv, "Teams", func() (string, error) { return output.Teams(res, module) })
		case "telegram":
			text, err := output.Telegram(res, module)
			if err != nil {
				return err
			}
			token, chat := os.Getenv(*tgTokenEnv), os.Getenv(*tgChatEnv)
			if token == "" || chat == "" {
				return fmt.Errorf("telegram not set: env %s and/or %s are empty", *tgTokenEnv, *tgChatEnv)
			}
			if err := postTelegram(context.Background(), token, chat, text); err != nil {
				return err
			}
			fmt.Println("checkfleet: report sent to Telegram")
		case "webhook":
			var payload string
			var err error
			if *tmplFile != "" {
				tmpl, rerr := os.ReadFile(*tmplFile)
				if rerr != nil {
					return fmt.Errorf("reading template %s: %w", *tmplFile, rerr)
				}
				payload, err = output.RenderTemplate(res, module, string(tmpl))
			} else {
				payload, err = output.JSON(res)
			}
			if err != nil {
				return err
			}
			url := os.Getenv(*webhookEnv)
			if url == "" {
				return fmt.Errorf("webhook not set: env %s is empty", *webhookEnv)
			}
			if err := postJSON(context.Background(), url, payload); err != nil {
				return err
			}
			fmt.Println("checkfleet: report sent to the webhook")
		default:
			rendered, err := render(sink, res, module, color)
			if err != nil {
				return err
			}
			if *outFile != "" {
				return atomicWrite(*outFile, rendered)
			}
			fmt.Print(rendered)
		}
		return nil
	}

	if len(sinks) == 1 {
		// One sink: errors abort the command (exit 1), as before.
		if err := emit(sinks[0]); err != nil {
			return err
		}
	} else {
		// Fan-out: each sink is isolated — a down webhook or an unset env doesn't
		// stop the others or fail the run (the finding gate is separate).
		for _, s := range sinks {
			if err := emit(s); err != nil {
				fmt.Fprintf(os.Stderr, "checkfleet: output %q: %v\n", s, err)
			}
		}
	}

	if *pingURLEnv != "" {
		if url := os.Getenv(*pingURLEnv); url != "" {
			if err := pingDeadman(context.Background(), url, engine.Worst(res.Findings)); err != nil {
				fmt.Fprintln(os.Stderr, "checkfleet: dead-man ping:", err)
			}
		}
	}

	if *exitOnBad {
		worst := engine.Worst(res.Findings)
		if worst == engine.BAD || worst == engine.ERROR {
			os.Exit(2)
		}
	}
	return nil
}

// runWatch re-runs the selected checks on an interval, redrawing a live text
// view until interrupted (Ctrl-C). Maintenance and filters apply each tick.
func runWatch(jobs []engine.Job, cfg *engine.Config, filter engine.FilterOptions, interval time.Duration, color bool, limit int) error {
	for {
		res := engine.RunJobsLimited(context.Background(), jobs, limit)
		res.Labels = cfg.Labels
		res.Findings = engine.ApplyMaintenance(res.Findings, cfg.Maintenance, time.Now())
		res.Findings = engine.Filter(res.Findings, filter)
		fmt.Print(watchFrame(res, time.Now(), interval, color))
		time.Sleep(interval)
	}
}

// watchFrame renders one live frame: clear the screen, a header, then the text
// output. Kept separate so it can be tested without the loop.
func watchFrame(res engine.Result, now time.Time, interval time.Duration, color bool) string {
	body := output.Text(res)
	if color {
		body = output.TextColor(res)
	}
	return fmt.Sprintf("\033[H\033[2Jcheckfleet — watch every %s — %s\n\n%s",
		interval, now.Format("15:04:05"), body)
}

// pingDeadman pings a dead-man's-switch URL (Healthchecks.io-style): the base
// URL on success, base+"/fail" when the worst finding is BAD/ERROR.
func pingDeadman(ctx context.Context, url string, worst engine.Status) error {
	if worst == engine.BAD || worst == engine.ERROR {
		url = strings.TrimRight(url, "/") + "/fail"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

// render turns a run into the printable output for a format (not slack).
func render(format string, res engine.Result, module string, color bool) (string, error) {
	switch format {
	case "text":
		if color {
			return output.TextColor(res), nil
		}
		return output.Text(res), nil
	case "markdown":
		return output.Markdown(res, module), nil
	case "json":
		s, err := output.JSON(res)
		return s + "\n", err
	case "junit":
		s, err := output.JUnit(res, module)
		return s + "\n", err
	case "html":
		return output.HTML(res, module), nil
	case "prometheus":
		return output.Prometheus(res), nil
	case "otlp":
		s, err := output.OTLP(res)
		return s + "\n", err
	case "csv":
		return output.CSV(res)
	default:
		return "", fmt.Errorf("unknown format %q", format)
	}
}

// isTerminal reports whether f is a character device (a terminal), so we only
// emit ANSI colour when a human is watching — never into a pipe or file.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// atomicWrite writes content to path via a temp file + rename, so a reader
// (e.g. the node_exporter textfile collector) never sees a partial file.
func atomicWrite(path, content string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".checkfleet-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

// runValidate checks the config without running any check; exit 1 if invalid.
//
//	checkfleet validate --config checkfleet.yml [--stack …]
func runValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	configPath := fs.String("config", "checkfleet.yml", "YAML config file")
	stack := fs.String("stack", "", "comma-separated stack profiles overlaid in order (last wins): checkfleet.<stack>.yml onto the base")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := loadConfig(*configPath, *stack)
	if err != nil {
		return err // parse/read errors are already fatal
	}
	problems := engine.Validate(cfg)
	if len(problems) == 0 {
		fmt.Printf("checkfleet: %s is valid ✅\n", *configPath)
		return nil
	}
	fmt.Fprintf(os.Stderr, "%s: %d problem(s):\n", *configPath, len(problems))
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
			Message: fmt.Sprintf("flapping: %d state changes in the last %d runs (now %s)", fl.Changes, len(recent), fl.Last),
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

// effectiveConcurrency resolves the global concurrency cap: a --max-concurrency
// flag (>= 0) wins over the config's max_concurrency (CF-116). -1 means the flag
// was not set. 0 = unbounded.
func effectiveConcurrency(flag int, cfg *engine.Config) int {
	if flag >= 0 {
		return flag
	}
	return cfg.MaxConcurrency
}

// loadConfig loads the base config, overlaying stack profiles when set. --stack
// accepts a comma-separated list applied left-to-right (last wins), e.g.
// --stack region,env (CF-117).
func loadConfig(path, stack string) (*engine.Config, error) {
	stacks := splitStacks(stack)
	if len(stacks) == 0 {
		return engine.LoadConfig(path)
	}
	return engine.LoadConfigStacks(path, stacks)
}

// splitStacks parses a comma-separated --stack value into trimmed, non-empty
// names, preserving order.
func splitStacks(stack string) []string { return splitCSV(stack) }

// splitCSV splits a comma-separated flag value into trimmed, non-empty entries,
// preserving order (used for --stack and the multi-sink --output).
func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// runServe exposes the findings as Prometheus metrics, re-running the checks on
// an interval. checkfleet serve --config … --listen :9876 --interval 60s
func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	configPath := fs.String("config", "checkfleet.yml", "YAML config file")
	stack := fs.String("stack", "", "comma-separated stack profiles overlaid in order (last wins): checkfleet.<stack>.yml onto the base")
	listen := fs.String("listen", ":9876", "listen address")
	interval := fs.Duration("interval", 60*time.Second, "interval between check re-runs")
	logFormat := fs.String("log-format", "text", "log format: text or json (structured)")
	maxConc := fs.Int("max-concurrency", -1, "cap on checks running at once (0 = unbounded); overrides max_concurrency in the config")
	if err := fs.Parse(args); err != nil {
		return err
	}
	logger := newLogger(*logFormat)
	cfg, err := loadConfig(*configPath, *stack)
	if err != nil {
		return err
	}
	limit := effectiveConcurrency(*maxConc, cfg)
	jobs := registry.Jobs(cfg, runOptions(cfg))
	if len(jobs) == 0 {
		return fmt.Errorf("no module configured in %s", *configPath)
	}

	var mu sync.Mutex
	var latest engine.Result
	var ready atomic.Bool
	runOnce := func() {
		res := engine.RunJobsLimited(context.Background(), jobs, limit)
		res.Labels = cfg.Labels
		res.Findings = engine.ApplyMaintenance(res.Findings, cfg.Maintenance, time.Now())
		mu.Lock()
		latest = res
		mu.Unlock()
		ready.Store(true)
		sum := engine.Summarize(res.Findings)
		logger.Info("run complete",
			"duration_ms", res.Duration.Milliseconds(),
			"worst", string(engine.Worst(res.Findings)),
			"ok", sum[engine.OK], "warn", sum[engine.WARN],
			"bad", sum[engine.BAD], "error", sum[engine.ERROR])
	}
	logger.Info("serve start", "modules", len(jobs), "listen", *listen, "interval", interval.String())
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
		fmt.Fprint(w, output.SelfMetrics(res)) // metrics about checkfleet itself (CF-87)
	})
	// Liveness: the process is up. Readiness: the first run has completed, so
	// /metrics has real data — for k8s/nomad probes (CF-88).
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	})
	http.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if !ready.Load() {
			http.Error(w, "not ready: no run yet", http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintln(w, "ready")
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "checkfleet %s\n\nmetrics: /metrics\nhealth: /healthz /readyz\n%d modules, re-run every %s\n", version, len(jobs), *interval)
	})
	return http.ListenAndServe(*listen, nil)
}

// newLogger returns a structured logger writing to stderr: JSON when format is
// "json" (for log pipelines), otherwise the human-readable text handler.
func newLogger(format string) *slog.Logger {
	if format == "json" {
		return slog.New(slog.NewJSONHandler(os.Stderr, nil))
	}
	return slog.New(slog.NewTextHandler(os.Stderr, nil))
}

// postRendered renders a chat payload and POSTs it to the webhook URL taken
// from env var webhookEnv, printing a confirmation. Shared by discord/teams.
func postRendered(webhookEnv, name string, render func() (string, error)) error {
	payload, err := render()
	if err != nil {
		return err
	}
	url := os.Getenv(webhookEnv)
	if url == "" {
		return fmt.Errorf("%s webhook not set: env %s is empty", name, webhookEnv)
	}
	if err := postJSON(context.Background(), url, payload); err != nil {
		return err
	}
	fmt.Printf("checkfleet: report sent to %s\n", name)
	return nil
}

// postJSON POSTs a JSON payload to a webhook URL, accepting any 2xx response.
// postTelegram sends a plain-text message via the Telegram Bot API sendMessage.
func postTelegram(ctx context.Context, token, chatID, text string) error {
	payload, err := json.Marshal(map[string]string{"chat_id": chatID, "text": text})
	if err != nil {
		return err
	}
	url := "https://api.telegram.org/bot" + token + "/sendMessage"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending to Telegram: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram responded HTTP %d", resp.StatusCode)
	}
	return nil
}

func postJSON(ctx context.Context, url, payload string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBufferString(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending to the webhook: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("the webhook responded HTTP %d", resp.StatusCode)
	}
	return nil
}
