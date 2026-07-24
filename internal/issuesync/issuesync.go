// Package issuesync reconciles a run's findings with a tracker's issues: it
// opens an issue for each BAD/ERROR finding (deduped by check+target) and
// closes the ones whose finding has recovered. The tracker is abstracted
// behind Client so the reconcile logic is unit-tested with a fake; a
// gh-backed client lives in the CLI.
package issuesync

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// titlePrefix marks issues managed by checkfleet and encodes the dedup key.
const titlePrefix = "[checkfleet] "

// Issue is an existing tracker issue managed by checkfleet.
type Issue struct {
	Number int
	Key    string // the "check/target" key parsed from its title
}

// Client is the subset of a tracker (GitHub, …) that issuesync needs.
type Client interface {
	List(ctx context.Context) ([]Issue, error)
	Create(ctx context.Context, title, body string) error
	Close(ctx context.Context, number int, comment string) error
}

// Report summarises what a reconcile did (or would do, in dry-run).
type Report struct {
	Created   []string // keys of opened issues
	Closed    []string // keys of closed (recovered) issues
	Unchanged int
}

// Key is the stable dedup identity of a finding: check + target.
func Key(f engine.Finding) string { return f.Check + "/" + f.Target }

// isProblem reports whether a finding warrants an issue.
func isProblem(s engine.Status) bool { return s == engine.BAD || s == engine.ERROR }

// title/keyFromTitle round-trip the dedup key through the issue title.
func title(f engine.Finding) string {
	return fmt.Sprintf("%s%s — %s", titlePrefix, Key(f), f.Status)
}

// KeyFromTitle extracts the "check/target" key from a managed issue title, or
// "" if the title isn't one of ours.
func KeyFromTitle(t string) string {
	if !strings.HasPrefix(t, titlePrefix) {
		return ""
	}
	rest := strings.TrimPrefix(t, titlePrefix)
	if i := strings.Index(rest, " — "); i >= 0 {
		return rest[:i]
	}
	return rest
}

// Reconcile opens issues for current problems missing one and closes managed
// issues whose problem has recovered. It is idempotent. With dryRun, it reports
// the actions without calling Create/Close.
func Reconcile(ctx context.Context, c Client, findings []engine.Finding, dryRun bool) (Report, error) {
	existing, err := c.List(ctx)
	if err != nil {
		return Report{}, err
	}
	open := map[string]Issue{}
	for _, is := range existing {
		if is.Key != "" {
			open[is.Key] = is
		}
	}

	// Current problems (worst finding per key), deduped.
	problems := map[string]engine.Finding{}
	for _, f := range findings {
		if isProblem(f.Status) {
			problems[Key(f)] = f
		}
	}

	var rep Report
	for _, key := range sortedKeys(problems) {
		if _, ok := open[key]; ok {
			rep.Unchanged++
			continue
		}
		rep.Created = append(rep.Created, key)
		if dryRun {
			continue
		}
		f := problems[key]
		if err := c.Create(ctx, title(f), body(f)); err != nil {
			return rep, err
		}
	}
	for _, key := range sortedIssueKeys(open) {
		if _, stillBad := problems[key]; stillBad {
			continue
		}
		rep.Closed = append(rep.Closed, key)
		if dryRun {
			continue
		}
		if err := c.Close(ctx, open[key].Number, "Recovered: checkfleet no longer reports this problem."); err != nil {
			return rep, err
		}
	}
	return rep, nil
}

func body(f engine.Finding) string {
	return fmt.Sprintf("**%s** — check `%s`, target `%s`\n\n> %s\n\n---\nAperta automaticamente da `checkfleet report-issues`. "+
		"Si chiude da sola quando il finding rientra. Non chiuderla a mano se il problema persiste.", f.Status, f.Check, f.Target, f.Message)
}

func sortedKeys(m map[string]engine.Finding) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedIssueKeys(m map[string]Issue) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
