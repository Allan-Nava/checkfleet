package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/Allan-Nava/checkfleet/internal/issuesync"
)

// issueClient builds the tracker client for a forge ("github" or "gitlab") and
// returns it with an ensureLabel closure (best-effort label creation).
func issueClient(forge, label string) (issuesync.Client, func(), error) {
	switch forge {
	case "github":
		c := &ghIssueClient{label: label}
		return c, c.ensureLabel, nil
	case "gitlab":
		c := &glIssueClient{label: label}
		return c, c.ensureLabel, nil
	default:
		return nil, nil, fmt.Errorf("unknown forge %q (github|gitlab)", forge)
	}
}

// glIssueClient implements issuesync.Client via the glab CLI (GitLab), mirroring
// the gh adapter. Reconcile logic is shared and unit-tested; this is thin glue.
type glIssueClient struct{ label string }

func (g *glIssueClient) List(context.Context) ([]issuesync.Issue, error) {
	out, err := glabRun("issue", "list", "--label", g.label, "--per-page", "500", "-F", "json")
	if err != nil {
		return nil, err
	}
	var raw []struct {
		IID   int    `json:"iid"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, err
	}
	var issues []issuesync.Issue
	for _, r := range raw {
		issues = append(issues, issuesync.Issue{Number: r.IID, Key: issuesync.KeyFromTitle(r.Title)})
	}
	return issues, nil
}

func (g *glIssueClient) Create(_ context.Context, title, body string) error {
	_, err := glabRun("issue", "create", "--title", title, "--description", body, "--label", g.label, "--yes")
	return err
}

func (g *glIssueClient) Close(_ context.Context, number int, comment string) error {
	// glab has no --comment on close; add a note first, then close.
	if _, err := glabRun("issue", "note", strconv.Itoa(number), "-m", comment); err != nil {
		return err
	}
	_, err := glabRun("issue", "close", strconv.Itoa(number))
	return err
}

func (g *glIssueClient) ensureLabel() {
	_, _ = glabRun("label", "create", "--name", g.label, "--color", "#b60205", "--description", "checkfleet BAD/ERROR finding (managed by report-issues)")
}

func glabRun(args ...string) (string, error) {
	cmd := exec.Command("glab", args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("glab %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}
