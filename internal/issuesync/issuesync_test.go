package issuesync

import (
	"context"
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// fakeClient records the reconcile actions instead of hitting a tracker.
type fakeClient struct {
	existing []Issue
	created  []string // titles
	closed   []int    // numbers
}

func (f *fakeClient) List(context.Context) ([]Issue, error) { return f.existing, nil }
func (f *fakeClient) Create(_ context.Context, title, _ string) error {
	f.created = append(f.created, title)
	return nil
}
func (f *fakeClient) Close(_ context.Context, number int, _ string) error {
	f.closed = append(f.closed, number)
	return nil
}

func find(check, target string, s engine.Status) engine.Finding {
	return engine.Finding{Check: check, Target: target, Status: s, Message: "msg"}
}

func TestKeyRoundTrip(t *testing.T) {
	f := find("http", "https://x/health", engine.BAD)
	if k := KeyFromTitle(title(f)); k != Key(f) {
		t.Errorf("round-trip chiave: %q != %q", k, Key(f))
	}
	if KeyFromTitle("issue non gestita") != "" {
		t.Error("una issue non-checkfleet non deve produrre una chiave")
	}
}

func TestOpensForBadAndError(t *testing.T) {
	c := &fakeClient{}
	findings := []engine.Finding{
		find("http", "a", engine.BAD),
		find("dns", "b", engine.ERROR),
		find("certs", "c", engine.OK),  // niente issue
		find("nats", "d", engine.WARN), // niente issue
	}
	rep, err := Reconcile(context.Background(), c, findings, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.created) != 2 || len(rep.Created) != 2 {
		t.Errorf("attese 2 issue create (BAD+ERROR), avute %v", c.created)
	}
}

func TestDedupExistingStaysOpen(t *testing.T) {
	c := &fakeClient{existing: []Issue{{Number: 7, Key: "http/a"}}}
	rep, err := Reconcile(context.Background(), c, []engine.Finding{find("http", "a", engine.BAD)}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.created) != 0 || rep.Unchanged != 1 {
		t.Errorf("problema già tracciato: niente create, unchanged=1; avuto created=%v unchanged=%d", c.created, rep.Unchanged)
	}
}

func TestClosesRecovered(t *testing.T) {
	c := &fakeClient{existing: []Issue{{Number: 9, Key: "http/a"}}}
	// Nessun problema corrente → l'issue va chiusa.
	rep, err := Reconcile(context.Background(), c, []engine.Finding{find("http", "a", engine.OK)}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.closed) != 1 || c.closed[0] != 9 || len(rep.Closed) != 1 {
		t.Errorf("recovery: attesa chiusura #9, avuto closed=%v", c.closed)
	}
}

func TestDryRunTouchesNothing(t *testing.T) {
	c := &fakeClient{existing: []Issue{{Number: 1, Key: "old/x"}}}
	rep, err := Reconcile(context.Background(), c, []engine.Finding{find("http", "new", engine.BAD)}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.created) != 0 || len(c.closed) != 0 {
		t.Errorf("dry-run non deve chiamare Create/Close, avuto created=%v closed=%v", c.created, c.closed)
	}
	if len(rep.Created) != 1 || len(rep.Closed) != 1 {
		t.Errorf("dry-run deve comunque riportare le azioni: %+v", rep)
	}
}
