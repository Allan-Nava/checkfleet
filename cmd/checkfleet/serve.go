// The `serve` command: a long-running Prometheus exporter.

package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/output"
	"github.com/Allan-Nava/checkfleet/internal/registry"
)

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
	warnUnknownKeys(os.Stderr, *configPath, *stack)
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
		res = engine.PostProcess(res, cfg, time.Now())
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
