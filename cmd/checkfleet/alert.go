package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/alert"
	"github.com/Allan-Nava/checkfleet/internal/awssig"
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
	provider := fs.String("provider", "pagerduty", "on-call provider: pagerduty, opsgenie or sns")
	keyEnv := fs.String("key-env", "", "env var with the PagerDuty routing key or Opsgenie API key")
	snsTopic := fs.String("sns-topic-arn", "", "SNS topic ARN (sns provider)")
	awsAccessEnv := fs.String("aws-access-key-env", "AWS_ACCESS_KEY_ID", "env var with the AWS access key id (sns provider)")
	awsSecretEnv := fs.String("aws-secret-key-env", "AWS_SECRET_ACCESS_KEY", "env var with the AWS secret access key (sns provider)")
	historyPath := fs.String("history", "", "JSONL history: resolve alerts that recovered since the previous run")
	statePath := fs.String("alert-state", "", "JSON file remembering what was last notified, for renotify_after (CF-176)")
	source := fs.String("source", "checkfleet", "alert source label (PagerDuty)")
	dryRun := fs.Bool("dry-run", false, "print the events without sending")
	if err := fs.Parse(args); err != nil {
		return err
	}
	// Check the provider name before running anything. Until CF-157 a typo was
	// only caught by sendAlert, so it surfaced *after* a full fleet sweep — and
	// not at all under --dry-run, which is exactly where you go looking for it.
	switch *provider {
	case "pagerduty", "opsgenie", "sns":
	default:
		return fmt.Errorf("unknown provider %q (pagerduty|opsgenie|sns)", *provider)
	}

	cfg, err := loadConfig(*configPath, *stack)
	if err != nil {
		return err
	}
	warnUnknownKeys(os.Stderr, *configPath, *stack)
	checks := registry.Configured(cfg)
	if len(checks) == 0 {
		return fmt.Errorf("no module configured in %s", *configPath)
	}
	ctx := context.Background()
	res := engine.RunWith(ctx, checks, runOptions(cfg))

	prevKeys := prevProblemKeys(*historyPath, res)
	events := alert.Plan(res.Findings, prevKeys)

	// Routing (CF-175): with alert_routes in the config, each event goes to the
	// provider its rule names. Without them the flags below behave exactly as
	// before, so nobody who has not asked for this sees a change.
	if len(cfg.AlertRoutes) > 0 {
		return sendRouted(ctx, cfg, events, res.Labels, *source, *statePath, *dryRun)
	}

	// SNS is a stateless pub/sub sink: it needs a topic ARN + AWS creds and only
	// publishes triggers (there is nothing to "resolve").
	var ak, sk string
	key := os.Getenv(*keyEnv)
	if *provider == "sns" {
		ak, sk = os.Getenv(*awsAccessEnv), os.Getenv(*awsSecretEnv)
		if !*dryRun && (*snsTopic == "" || ak == "" || sk == "") {
			return fmt.Errorf("sns needs --sns-topic-arn and env %s/%s", *awsAccessEnv, *awsSecretEnv)
		}
	} else if key == "" && !*dryRun {
		return fmt.Errorf("alert key not set: env %s is empty", *keyEnv)
	}

	for _, e := range events {
		if *dryRun {
			fmt.Printf("  %-7s %s\n", e.Action, e.DedupKey)
			continue
		}
		if *provider == "sns" {
			if e.Action != "trigger" {
				continue // SNS has no resolve
			}
			if err := sendSNS(ctx, *snsTopic, ak, sk, e); err != nil {
				return err
			}
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

// sendSNS publishes one event to an SNS topic, signing the request with SigV4.
func sendSNS(ctx context.Context, topicArn, accessKey, secretKey string, e alert.Event) error {
	region := alert.RegionFromTopicARN(topicArn)
	if region == "" {
		return fmt.Errorf("cannot parse region from topic ARN %q", topicArn)
	}
	body := alert.SNSForm(topicArn, e.DedupKey, e.Summary)
	url := "https://sns." + region + ".amazonaws.com/"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	awssig.Sign(req, []byte(body), accessKey, secretKey, region, "sns", time.Now())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("sns: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("sns responded HTTP %d", resp.StatusCode)
	}
	return nil
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

// sendRouted dispatches each event to the provider its route names (CF-175).
//
// An event that matches no route is reported and skipped rather than sent
// somewhere arbitrary: a config with rules is a config that has opinions about
// where things go, and quietly defaulting would deliver a database alert to
// whoever happens to be first in the list.
func sendRouted(ctx context.Context, cfg *engine.Config, events []alert.Event,
	labels map[string]string, source, statePath string, dryRun bool) error {

	state, err := alert.LoadState(statePath)
	if err != nil {
		return err
	}
	now := time.Now()

	var sent, unrouted, held int
	for _, e := range events {
		r, ok := alert.Match(cfg.AlertRoutes, e, labels)
		if !ok {
			unrouted++
			fmt.Fprintf(os.Stderr, "checkfleet: alert: no route for %s %s\n", e.Action, e.DedupKey)
			continue
		}
		// A resolve always goes, and clears the memory so the next occurrence
		// is a first notification rather than an old timer.
		if e.Action == "resolve" {
			state.Forget(e.DedupKey)
		} else if send, why := alert.Decide(state.Sent[e.DedupKey], e.Severity, now, policyOf(r)); !send {
			held++
			if dryRun {
				fmt.Printf("  %-7s %-40s · held (%s)\n", e.Action, e.DedupKey, why)
			}
			continue
		}
		if dryRun {
			// The routing decision is the thing you want to see before turning
			// this on, so --dry-run names the provider per event.
			fmt.Printf("  %-7s %-40s → %s\n", e.Action, e.DedupKey, routeLabel(r))
			sent++
			continue
		}
		if err := sendVia(ctx, r, source, e); err != nil {
			return err
		}
		if e.Action == "trigger" {
			state.Record(e.DedupKey, now, e.Severity)
		}
		sent++
	}
	if !dryRun {
		if err := alert.SaveState(statePath, state); err != nil {
			return err
		}
	}
	fmt.Printf("alert: %d events routed, %d held, %d unrouted (dry-run=%v)\n", sent, held, unrouted, dryRun)
	return nil
}

// policyOf reads the re-notification policy off a route (CF-176). An
// unparseable duration was already reported by validate, so it degrades to
// "never" here rather than aborting a run that is trying to page someone.
func policyOf(r engine.AlertRoute) alert.Policy {
	p := alert.Policy{OnWorsening: r.RenotifyOnWorsening}
	if r.RenotifyAfter != "" {
		if d, err := time.ParseDuration(r.RenotifyAfter); err == nil {
			p.After = d
		}
	}
	return p
}

// routeLabel describes a route for the dry-run line.
func routeLabel(r engine.AlertRoute) string {
	if r.Provider == "sns" {
		return "sns " + r.SNSTopicARN
	}
	return r.Provider + " (" + r.KeyEnv + ")"
}

// sendVia delivers one event through one route.
func sendVia(ctx context.Context, r engine.AlertRoute, source string, e alert.Event) error {
	if r.Provider == "sns" {
		if e.Action != "trigger" {
			return nil // SNS has no resolve
		}
		ak, sk := os.Getenv("AWS_ACCESS_KEY_ID"), os.Getenv("AWS_SECRET_ACCESS_KEY")
		return sendSNS(ctx, r.SNSTopicARN, ak, sk, e)
	}
	key := os.Getenv(r.KeyEnv)
	if key == "" {
		return fmt.Errorf("route %s: env %s is empty", r.Provider, r.KeyEnv)
	}
	return sendAlert(ctx, r.Provider, key, source, e)
}
