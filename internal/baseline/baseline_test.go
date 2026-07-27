package baseline

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

func f(check, target string, s engine.Status) engine.Finding {
	return engine.Finding{Check: check, Target: target, Status: s, Message: string(s)}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")
	now := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)
	findings := []engine.Finding{
		f("certs", "a:443", engine.BAD),
		f("http", "https://ok/", engine.OK),
	}
	if err := Save(path, findings, now); err != nil {
		t.Fatal(err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != currentVersion {
		t.Errorf("version = %d, want %d", got.Version, currentVersion)
	}
	if !got.Recorded.Equal(now) {
		t.Errorf("recorded = %v, want %v", got.Recorded, now)
	}
	// OK entries must be kept: without them a target going OK → BAD could not
	// be told apart from a target the baseline never saw.
	if len(got.Entries) != 2 {
		t.Fatalf("got %d entries, want 2 (OK findings included)", len(got.Entries))
	}
	m := got.StatusMap()
	if m["certs\ta:443"] != engine.BAD || m["http\thttps://ok/"] != engine.OK {
		t.Errorf("status map wrong: %v", m)
	}
}

func TestLoadRejectsUnknownVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")
	if err := os.WriteFile(path, []byte(`{"version":99,"entries":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Error("a future baseline version must be rejected, not silently misread")
	}
}

func TestLoadRejectsGarbage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Error("an unparseable baseline must be an error")
	}
}

func TestNewOrWorse(t *testing.T) {
	base := File{Version: currentVersion, Entries: []Entry{
		{Check: "certs", Target: "known-bad:443", Status: "BAD"},
		{Check: "certs", Target: "known-warn:443", Status: "WARN"},
		{Check: "http", Target: "https://fine/", Status: "OK"},
	}}

	tests := []struct {
		name    string
		current []engine.Finding
		want    []string // targets expected to be reported
	}{
		{
			name:    "known debt is tolerated",
			current: []engine.Finding{f("certs", "known-bad:443", engine.BAD)},
			want:    nil,
		},
		{
			name:    "a target the baseline never saw is new",
			current: []engine.Finding{f("certs", "fresh:443", engine.BAD)},
			want:    []string{"fresh:443"},
		},
		{
			name:    "a regression on a known-imperfect target still counts",
			current: []engine.Finding{f("certs", "known-warn:443", engine.BAD)},
			want:    []string{"known-warn:443"},
		},
		{
			name:    "improvement is not a failure",
			current: []engine.Finding{f("certs", "known-bad:443", engine.WARN)},
			want:    nil,
		},
		{
			name:    "a healthy target going bad is new",
			current: []engine.Finding{f("http", "https://fine/", engine.BAD)},
			want:    []string{"https://fine/"},
		},
		{
			name:    "still-green targets are silent",
			current: []engine.Finding{f("http", "https://fine/", engine.OK)},
			want:    nil,
		},
		{
			// Same target string, different module: they are distinct findings.
			name:    "the key includes the check name",
			current: []engine.Finding{f("tls", "known-bad:443", engine.BAD)},
			want:    []string{"known-bad:443"},
		},
		{
			name: "mixed run reports only the regressions",
			current: []engine.Finding{
				f("certs", "known-bad:443", engine.BAD),  // tolerated
				f("certs", "known-warn:443", engine.BAD), // worsened
				f("certs", "fresh:443", engine.WARN),     // new
				f("http", "https://fine/", engine.OK),    // fine
			},
			want: []string{"fresh:443", "known-warn:443"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewOrWorse(tt.current, base)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d findings %v, want %v", len(got), targets(got), tt.want)
			}
			seen := map[string]bool{}
			for _, x := range got {
				seen[x.Target] = true
			}
			for _, w := range tt.want {
				if !seen[w] {
					t.Errorf("missing %q; got %v", w, targets(got))
				}
			}
		})
	}
}

// An empty baseline excuses nothing: every non-OK finding is new.
func TestNewOrWorseAgainstEmptyBaseline(t *testing.T) {
	got := NewOrWorse([]engine.Finding{
		f("certs", "a:443", engine.BAD),
		f("http", "https://ok/", engine.OK),
	}, File{Version: currentVersion})

	if len(got) != 1 || got[0].Target != "a:443" {
		t.Errorf("got %v, want just the BAD finding", targets(got))
	}
}

// A finding that disappears from the run must not be reported as new.
func TestNewOrWorseIgnoresResolved(t *testing.T) {
	base := File{Version: currentVersion, Entries: []Entry{
		{Check: "certs", Target: "gone:443", Status: "BAD"},
	}}
	if got := NewOrWorse(nil, base); len(got) != 0 {
		t.Errorf("a resolved/absent finding was reported: %v", targets(got))
	}
}

func targets(fs []engine.Finding) []string {
	out := make([]string, 0, len(fs))
	for _, x := range fs {
		out = append(out, x.Target)
	}
	return out
}
