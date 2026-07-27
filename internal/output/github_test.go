package output

import (
	"strings"
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

func githubResult() engine.Result {
	return engine.Result{
		Findings: []engine.Finding{
			{Check: "certs", Target: "api.example.com:443", Status: engine.BAD, Message: "expires in 3 days"},
			{Check: "http", Target: "https://example.com/", Status: engine.ERROR, Message: "dial tcp: timeout"},
			{Check: "http", Target: "https://example.com/slow", Status: engine.WARN, Message: "slow: 900ms"},
			{Check: "http", Target: "https://example.com/ok", Status: engine.OK, Message: "HTTP 200, 40ms"},
		},
		Duration: 1500 * time.Millisecond,
	}
}

func TestGitHubLevels(t *testing.T) {
	got := GitHub(githubResult())
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")

	if len(lines) != 3 {
		t.Fatalf("got %d annotations, want 3 (OK must be skipped)\n%s", len(lines), got)
	}
	for _, want := range []string{
		"::error title=checkfleet certs%3A api.example.com%3A443 [BAD]::expires in 3 days",
		// A colon inside the *message* stays literal — only property values
		// need it escaped.
		"::error title=checkfleet http%3A https%3A//example.com/ [ERROR]::dial tcp: timeout",
		"::warning title=checkfleet http%3A https%3A//example.com/slow [WARN]::slow: 900ms",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing annotation:\n  want %q\n  got:\n%s", want, got)
		}
	}
	if strings.Contains(got, "/ok") {
		t.Errorf("OK finding was annotated; annotations are for attention only:\n%s", got)
	}
}

func TestGitHubEscaping(t *testing.T) {
	// A colon in a *message* is legal and must survive; only properties need it
	// escaped. Percent must be escaped first or it would corrupt the others.
	tests := []struct {
		name string
		in   string
		data string // want from escapeData
		prop string // want from escapeProperty
	}{
		{"percent", "100% full", "100%25 full", "100%25 full"},
		{"colon", "host:443", "host:443", "host%3A443"},
		{"comma", "a,b", "a,b", "a%2Cb"},
		{"newline", "line1\nline2", "line1%0Aline2", "line1%0Aline2"},
		{"carriage return", "a\rb", "a%0Db", "a%0Db"},
		// The ordering trap: escaping "%" last would turn the "%0A" produced by
		// the newline rule into "%250A".
		{"percent then newline", "%\n", "%25%0A", "%25%0A"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := escapeData(tt.in); got != tt.data {
				t.Errorf("escapeData(%q) = %q, want %q", tt.in, got, tt.data)
			}
			if got := escapeProperty(tt.in); got != tt.prop {
				t.Errorf("escapeProperty(%q) = %q, want %q", tt.in, got, tt.prop)
			}
		})
	}
}

func TestGitHubAllGreenEmitsNothing(t *testing.T) {
	res := engine.Result{Findings: []engine.Finding{
		{Check: "http", Target: "https://example.com/", Status: engine.OK, Message: "HTTP 200"},
	}}
	if got := GitHub(res); got != "" {
		t.Errorf("all-green run produced annotations, want none:\n%s", got)
	}
}

func TestGitHubSummaryMatchesMarkdown(t *testing.T) {
	res := githubResult()
	if GitHubSummary(res, "http") != Markdown(res, "http") {
		t.Error("the job summary and the markdown sink disagree about the same run")
	}
}
