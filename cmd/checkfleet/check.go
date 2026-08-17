// The `check` command: run the selected modules, then filter, gate, record and
// render the result.

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/baseline"
	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/history"
	"github.com/Allan-Nava/checkfleet/internal/insight"
	"github.com/Allan-Nava/checkfleet/internal/registry"
)

func runCheck(args []string) error {
	if len(args) < 1 {
		usage()
		return fmt.Errorf("missing module")
	}
	module := args[0]

	fs := flag.NewFlagSet("check", flag.ExitOnError)
	configPath := fs.String("config", "checkfleet.yml", "YAML config file")
	stack := fs.String("stack", "", "comma-separated stack profiles overlaid in order (last wins): checkfleet.<stack>.yml onto the base")
	format := fs.String("output", "text", "output sink(s), comma-separated to fan out (e.g. text,slack): text, markdown, json, junit, html, github, sarif, prometheus, otlp, csv, slack, discord, teams, telegram, webhook")
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
	exitOnBad := fs.Bool("exit-on-bad", false, "alias of --exit-on bad")
	exitOn := fs.String("exit-on", "", "severity that fails the build: warn|bad|error (empty = never fail on findings)")
	exitCode := fs.Int("exit-code", defaultExitCode, "exit code to use when --exit-on trips (1-125)")
	maxConc := fs.Int("max-concurrency", -1, "cap on checks running at once (0 = unbounded); overrides max_concurrency in the config")
	baselinePath := fs.String("baseline", "", "baseline file of known findings; created on first use")
	failOnNew := fs.Bool("fail-on-new", false, "gate only on findings absent from --baseline or worse than it (implies --exit-on bad)")
	writeBaseline := fs.Bool("write-baseline", false, "overwrite --baseline with this run's findings and skip the gate")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	minSev, ok := engine.ParseStatus(*minSeverity)
	if !ok {
		return fmt.Errorf("--min-severity %q is not valid (use ok|warn|bad|error)", *minSeverity)
	}
	filter := engine.FilterOptions{Only: commaSet(*only), MinSeverity: minSev, TargetGlob: *targetGlob}

	// Parsed before the run, not after: a typo in --exit-on should cost you a
	// usage error, not a full fleet sweep followed by one.
	exitGate, err := parseGate(*exitOn, *exitOnBad, *exitCode)
	if err != nil {
		return err
	}
	exitGate = exitGate.withImpliedThreshold(*failOnNew)
	if (*failOnNew || *writeBaseline) && *baselinePath == "" {
		return fmt.Errorf("--fail-on-new and --write-baseline need --baseline FILE")
	}

	cfg, err := loadConfig(*configPath, *stack)
	if err != nil {
		return err
	}
	warnUnknownKeys(os.Stderr, *configPath, *stack)

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
		// Compaction runs after this run is written, so the newest record is
		// never the one thinned away (CF-177). A failure here is reported and
		// not fatal: a history that grew too large is a smaller problem than a
		// check run that refused to finish.
		if cfg.HistoryRetention.Set() {
			if dropped, err := compactHistory(*historyPath, cfg.HistoryRetention); err != nil {
				fmt.Fprintln(os.Stderr, "checkfleet: history retention:", err)
			} else if dropped > 0 {
				fmt.Fprintf(os.Stderr, "checkfleet: history: %d old record(s) compacted\n", dropped)
			}
		}
	}
	res = engine.PostProcess(res, cfg, time.Now())
	res.Findings = engine.Filter(res.Findings, filter)

	// With a history in hand, the run carries the M30 analyses instead of
	// requiring a second `checkfleet insight` invocation (CF-173). Only the
	// analyses that need nothing from the operator: a forecast needs a threshold
	// and a budget needs an objective, and guessing either would be worse than
	// leaving them to the dedicated command.
	var report *insight.Report
	if *historyPath != "" && !*diff {
		if recent, err := history.Open(*historyPath).Recent(*flapWindow); err == nil && len(recent) > 0 {
			r := insight.Analyse(recent, res.Findings, insight.DefaultOptions(time.Now()))
			if !r.Empty() {
				report = &r
			}
		}
	}

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

	if err := emitAll(sinks, res, sinkOptions{
		renderCtx:  renderCtx{module: module, color: color, configPath: *configPath, insight: report},
		outFile:    *outFile,
		webhookEnv: *webhookEnv,
		tgTokenEnv: *tgTokenEnv,
		tgChatEnv:  *tgChatEnv,
		tmplFile:   *tmplFile,
	}); err != nil {
		return err
	}

	if *pingURLEnv != "" {
		if url := os.Getenv(*pingURLEnv); url != "" {
			if err := pingDeadman(context.Background(), url, engine.Worst(res.Findings)); err != nil {
				fmt.Fprintln(os.Stderr, "checkfleet: dead-man ping:", err)
			}
		}
	}

	// The baseline runs after the output so the report is emitted either way,
	// and before the gate because it decides *which* findings the gate sees.
	gated, skipGate, err := applyBaseline(*baselinePath, *writeBaseline, *failOnNew, res.Findings)
	if err != nil {
		return err
	}
	if skipGate {
		return nil
	}

	if code := exitGate.exitCode(engine.Worst(gated)); code != 0 {
		os.Exit(code)
	}
	return nil
}

// applyBaseline records or consults the baseline file, returning the findings
// the gate should judge. It reports skipGate when this run only recorded a
// baseline: there is nothing to compare against yet, so failing the build on
// the very run that captured the debt would defeat the purpose.
func applyBaseline(path string, write, failOnNew bool, findings []engine.Finding) (gated []engine.Finding, skipGate bool, err error) {
	if path == "" {
		return findings, false, nil
	}
	_, statErr := os.Stat(path)
	switch {
	case write, os.IsNotExist(statErr):
		if err := baseline.Save(path, findings, time.Now()); err != nil {
			return nil, false, err
		}
		fmt.Fprintf(os.Stderr, "checkfleet: baseline recorded in %s (%d findings); the gate is skipped for this run\n",
			path, len(findings))
		return findings, true, nil
	case statErr != nil:
		return nil, false, statErr
	}

	base, err := baseline.Load(path)
	if err != nil {
		return nil, false, err
	}
	if !failOnNew {
		// --baseline alone is inert on the gate: it takes --fail-on-new to
		// narrow it. Keeping the two separate means adding a baseline to a
		// pipeline can never quietly loosen an existing gate.
		return findings, false, nil
	}
	fresh := baseline.NewOrWorse(findings, base)
	fmt.Fprintf(os.Stderr, "checkfleet: %d finding(s) new or worse than the baseline recorded %s\n",
		len(fresh), base.Recorded.Format(time.RFC3339))
	return fresh, false, nil
}

// recordHistory appends this run to the JSONL history and returns WARN
// findings for keys that are flapping across the recent window.
func recordHistory(path string, res engine.Result, minChanges, window int) ([]engine.Finding, error) {
	store := history.Open(path)
	rec := history.Record{Unix: res.Started.Unix()}
	for _, f := range res.Findings {
		// Value/Unit ride along: they are what makes a metric chartable over time
		// (CF-91). Dropping them here — as this did until CF-157 — meant a history
		// written by the CLI carried no metric series at all, while one written by
		// the desktop did, from the same documented format.
		rec.Entries = append(rec.Entries, history.Entry{
			Check: f.Check, Target: f.Target, Status: string(f.Status),
			Value: f.Value, Unit: f.Unit,
		})
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

// compactHistory applies the configured retention to the history file (CF-177).
// The durations were validated at load time; an unparseable one here degrades
// to "no limit" rather than aborting a run that has already done its work.
func compactHistory(path string, r engine.HistoryRetention) (int, error) {
	p := history.RetentionPolicy{MaxRuns: r.MaxRuns}
	if r.MaxAge != "" {
		if d, err := time.ParseDuration(r.MaxAge); err == nil {
			p.MaxAge = d
		}
	}
	if r.DownsampleAfter != "" {
		if d, err := time.ParseDuration(r.DownsampleAfter); err == nil {
			p.DownsampleAfter = d
		}
	}
	return history.Open(path).Compact(p, time.Now())
}
