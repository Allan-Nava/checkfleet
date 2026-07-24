package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/issuesync"
)

const issueLabel = "checkfleet-finding"

// runReportIssues runs the checks and reconciles GitHub issues with the
// BAD/ERROR findings (open on failure, close on recovery, dedup by
// check+target). Requires the gh CLI, authenticated.
//
//	checkfleet report-issues --config checkfleet.yml [--stack …] [--dry-run]
func runReportIssues(args []string) error {
	fs := flag.NewFlagSet("report-issues", flag.ExitOnError)
	configPath := fs.String("config", "checkfleet.yml", "file di configurazione YAML")
	stack := fs.String("stack", "", "profilo stack: sovrappone checkfleet.<stack>.yml alla base")
	dryRun := fs.Bool("dry-run", false, "stampa le azioni senza toccare le issue")
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

	ctx := context.Background()
	res := engine.Run(ctx, checks, time.Duration(cfg.TimeoutSeconds)*time.Second)

	client := &ghIssueClient{label: issueLabel}
	if !*dryRun {
		client.ensureLabel()
	}
	rep, err := issuesync.Reconcile(ctx, client, res.Findings, *dryRun)
	if err != nil {
		return err
	}
	fmt.Printf("report-issues: %d aperte, %d chiuse, %d invariate (dry-run=%v)\n",
		len(rep.Created), len(rep.Closed), rep.Unchanged, *dryRun)
	for _, k := range rep.Created {
		fmt.Printf("  apro   %s\n", k)
	}
	for _, k := range rep.Closed {
		fmt.Printf("  chiudo %s\n", k)
	}
	return nil
}

// ghIssueClient implements issuesync.Client via the gh CLI.
type ghIssueClient struct{ label string }

func (g *ghIssueClient) List(context.Context) ([]issuesync.Issue, error) {
	out, err := ghRun("issue", "list", "--state", "open", "--label", g.label, "--limit", "500", "--json", "number,title")
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, err
	}
	var issues []issuesync.Issue
	for _, r := range raw {
		issues = append(issues, issuesync.Issue{Number: r.Number, Key: issuesync.KeyFromTitle(r.Title)})
	}
	return issues, nil
}

func (g *ghIssueClient) Create(_ context.Context, title, body string) error {
	_, err := ghRun("issue", "create", "--title", title, "--body", body, "--label", g.label)
	return err
}

func (g *ghIssueClient) Close(_ context.Context, number int, comment string) error {
	_, err := ghRun("issue", "close", strconv.Itoa(number), "--comment", comment)
	return err
}

func (g *ghIssueClient) ensureLabel() {
	_, _ = ghRun("label", "create", g.label, "--color", "b60205", "--description", "Finding BAD/ERROR di checkfleet (gestita da report-issues)")
}

func ghRun(args ...string) (string, error) {
	cmd := exec.Command("gh", args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("gh %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}
