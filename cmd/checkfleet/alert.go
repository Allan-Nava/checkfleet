package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/Allan-Nava/checkfleet/internal/alert"
	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/history"
	"github.com/Allan-Nava/checkfleet/internal/registry"
)

// runAlert runs the checks and creates/resolves on-call alerts for BAD/ERROR
// findings (dedup by check/target). With --history it resolves alerts that
// recovered since the previous run.
//
//	checkfleet alert --provider pagerduty --key-env PD_ROUTING_KEY [--history f]
func runAlert(args []string) error {
	fs := flag.NewFlagSet("alert", flag.ExitOnError)
	configPath := fs.String("config", "checkfleet.yml", "YAML config file")
	stack := fs.String("stack", "", "stack profile: overlays checkfleet.<stack>.yml onto the base")
	provider := fs.String("provider", "pagerduty", "on-call provider: pagerduty or opsgenie")
	keyEnv := fs.String("key-env", "", "env var with the PagerDuty routing key or Opsgenie API key")
	historyPath := fs.String("history", "", "JSONL history: resolve alerts that recovered since the previous run")
	source := fs.String("source", "checkfleet", "alert source label (PagerDuty)")
	dryRun := fs.Bool("dry-run", false, "print the events without sending")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := loadConfig(*configPath, *stack)
	if err != nil {
		return err
	}
	checks := registry.Configured(cfg)
	if len(checks) == 0 {
		return fmt.Errorf("no module configured in %s", *configPath)
	}
	ctx := context.Background()
	res := engine.RunWith(ctx, checks, runOptions(cfg))

	prevKeys := prevProblemKeys(*historyPath, res)
	events := alert.Plan(res.Findings, prevKeys)

	key := os.Getenv(*keyEnv)
	if key == "" && !*dryRun {
		return fmt.Errorf("alert key not set: env %s is empty", *keyEnv)
	}
	for _, e := range events {
		if *dryRun {
			fmt.Printf("  %-7s %s\n", e.Action, e.DedupKey)
			continue
		}
		if err := sendAlert(ctx, *provider, key, *source, e); err != nil {
			return err
		}
	}
	fmt.Printf("alert: %d events (provider=%s, dry-run=%v)\n", len(events), *provider, *dryRun)
	return nil
}

// prevProblemKeys returns the BAD/ERROR keys from the previous history run, then
// appends the current run (so the next invocation can resolve recoveries).
func prevProblemKeys(path string, res engine.Result) []string {
	if path == "" {
		return nil
	}
	store := history.Open(path)
	var keys []string
	if recent, err := store.Recent(1); err == nil && len(recent) > 0 {
		for _, e := range recent[0].Entries {
			if e.Status == string(engine.BAD) || e.Status == string(engine.ERROR) {
				keys = append(keys, e.Check+"/"+e.Target)
			}
		}
	}
	rec := history.Record{Unix: res.Started.Unix()}
	for _, f := range res.Findings {
		rec.Entries = append(rec.Entries, history.Entry{Check: f.Check, Target: f.Target, Status: string(f.Status)})
	}
	_ = store.Append(rec)
	return keys
}

// sendAlert posts one event to the provider.
func sendAlert(ctx context.Context, provider, key, source string, e alert.Event) error {
	switch provider {
	case "pagerduty":
		payload, err := alert.PagerDutyPayload(key, source, e)
		if err != nil {
			return err
		}
		return postJSON(ctx, "https://events.pagerduty.com/v2/enqueue", payload)
	case "opsgenie":
		return sendOpsgenie(ctx, key, e)
	default:
		return fmt.Errorf("unknown provider %q (pagerduty|opsgenie)", provider)
	}
}

// sendOpsgenie creates or closes an Opsgenie alert (alias = dedup key).
func sendOpsgenie(ctx context.Context, key string, e alert.Event) error {
	url := "https://api.opsgenie.com/v2/alerts"
	body := "{}"
	if e.Action == "trigger" {
		var err error
		if body, err = alert.OpsgenieCreatePayload(e); err != nil {
			return err
		}
	} else {
		url = fmt.Sprintf("%s/%s/close?identifierType=alias", url, e.DedupKey)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBufferString(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "GenieKey "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("opsgenie: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("opsgenie responded HTTP %d", resp.StatusCode)
	}
	return nil
}
