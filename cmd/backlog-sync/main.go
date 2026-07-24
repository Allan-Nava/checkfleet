// Command backlog-sync keeps GitHub issues in sync with BACKLOG.md.
//
// BACKLOG.md is the single source of truth: every CF-n item becomes an issue
// (label "backlog", grouped by milestone). Checking an item ([x]) closes its
// issue; unchecking reopens it. The sync is idempotent — issues are matched by
// their "CF-n" title prefix — so it is safe to run repeatedly (locally or from
// .github/workflows/backlog-sync.yml).
//
//	go run ./cmd/backlog-sync [-backlog BACKLOG.md] [-dry-run]
//
// Requires the `gh` CLI, authenticated (GH_TOKEN in CI).
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/Allan-Nava/checkfleet/internal/backlog"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "stampa le azioni senza eseguirle")
	path := flag.String("backlog", "BACKLOG.md", "percorso di BACKLOG.md")
	flag.Parse()

	raw, err := os.ReadFile(*path)
	if err != nil {
		fatal(err)
	}
	items := backlog.Parse(string(raw))
	if len(items) == 0 {
		fatal(fmt.Errorf("nessun item CF-n in %s", *path))
	}

	s := &syncer{dryRun: *dryRun}
	if err := s.run(items); err != nil {
		fatal(err)
	}
	fmt.Printf("backlog-sync: %d item · %d create, %d chiuse, %d riaperte, %d invariate (dry-run=%v)\n",
		len(items), s.created, s.closed, s.reopened, s.unchanged, *dryRun)
}

type syncer struct {
	dryRun                               bool
	created, closed, reopened, unchanged int
}

type ghIssue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"` // "OPEN" | "CLOSED"
}

func (s *syncer) run(items []backlog.Item) error {
	s.ensureLabel()
	if err := s.ensureMilestones(items); err != nil {
		return err
	}
	existing, err := s.listIssues()
	if err != nil {
		return err
	}
	for _, it := range items {
		if err := s.syncItem(it, existing[it.ID]); err != nil {
			return err
		}
	}
	return nil
}

func (s *syncer) syncItem(it backlog.Item, cur *ghIssue) error {
	if cur == nil {
		return s.createIssue(it)
	}
	switch {
	case it.Done && cur.State == "OPEN":
		s.closed++
		return s.act("chiudo", it, func() error {
			return ghVoid("issue", "close", strconv.Itoa(cur.Number), "--comment", "Completata: item spuntato in BACKLOG.md.")
		})
	case !it.Done && cur.State == "CLOSED":
		s.reopened++
		return s.act("riapro", it, func() error { return ghVoid("issue", "reopen", strconv.Itoa(cur.Number)) })
	default:
		s.unchanged++
		return nil
	}
}

func (s *syncer) createIssue(it backlog.Item) error {
	s.created++
	return s.act("creo", it, func() error {
		args := []string{"issue", "create", "--title", it.Title, "--body", issueBody(it), "--label", "backlog"}
		if it.Milestone != "" {
			args = append(args, "--milestone", it.Milestone)
		}
		out, err := gh(args...)
		if err != nil {
			return err
		}
		// A done item is recorded as a closed issue for full history.
		if it.Done {
			if n := lastPathInt(out); n > 0 {
				return ghVoid("issue", "close", strconv.Itoa(n), "--comment", "Item già completato in BACKLOG.md.")
			}
		}
		return nil
	})
}

// act logs the action and runs it unless in dry-run mode.
func (s *syncer) act(verb string, it backlog.Item, do func() error) error {
	fmt.Printf("  %-7s %s (%s)\n", verb, it.ID, it.Milestone)
	if s.dryRun {
		return nil
	}
	return do()
}

func (s *syncer) ensureLabel() {
	// Best-effort: fails harmlessly if the label already exists.
	if s.dryRun {
		return
	}
	_, _ = gh("label", "create", "backlog", "--color", "5319e7", "--description", "Item del BACKLOG.md (CF-n), sincronizzato da backlog-sync")
}

func (s *syncer) ensureMilestones(items []backlog.Item) error {
	out, err := gh("api", "repos/{owner}/{repo}/milestones?state=all", "--jq", ".[].title")
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line != "" {
			have[line] = true
		}
	}
	seen := map[string]bool{}
	for _, it := range items {
		if it.Milestone == "" || seen[it.Milestone] || have[it.Milestone] {
			continue
		}
		seen[it.Milestone] = true
		fmt.Printf("  milestone  %s\n", it.Milestone)
		if s.dryRun {
			continue
		}
		if _, err := gh("api", "-X", "POST", "repos/{owner}/{repo}/milestones", "-f", "title="+it.Milestone); err != nil {
			return err
		}
	}
	return nil
}

func (s *syncer) listIssues() (map[string]*ghIssue, error) {
	out, err := gh("issue", "list", "--state", "all", "--limit", "300", "--json", "number,title,state")
	if err != nil {
		return nil, err
	}
	var issues []ghIssue
	if err := json.Unmarshal([]byte(out), &issues); err != nil {
		return nil, err
	}
	byID := map[string]*ghIssue{}
	for i := range issues {
		if id := idFromTitle(issues[i].Title); id != "" {
			byID[id] = &issues[i]
		}
	}
	return byID, nil
}

func issueBody(it backlog.Item) string {
	return fmt.Sprintf("%s\n\n---\nTracciata da `BACKLOG.md` (**%s**) · milestone _%s_.\n\n"+
		"Gestita da `cmd/backlog-sync`: modifica il **BACKLOG**, non questa issue. "+
		"Spunta l'item (`[x]`) e la issue verrà chiusa al prossimo sync.", it.Description, it.ID, it.Milestone)
}

// idFromTitle returns the leading "CF-n" token of an issue title, or "".
func idFromTitle(title string) string {
	fields := strings.Fields(title)
	if len(fields) > 0 && strings.HasPrefix(fields[0], "CF-") {
		return fields[0]
	}
	return ""
}

// lastPathInt parses the trailing integer of the last URL printed by gh
// (e.g. https://github.com/owner/repo/issues/42 -> 42).
func lastPathInt(s string) int {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "/"); i >= 0 {
		if n, err := strconv.Atoi(strings.TrimSpace(s[i+1:])); err == nil {
			return n
		}
	}
	return 0
}

func gh(args ...string) (string, error) {
	cmd := exec.Command("gh", args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("gh %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

func ghVoid(args ...string) error {
	_, err := gh(args...)
	return err
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "backlog-sync:", err)
	os.Exit(1)
}
