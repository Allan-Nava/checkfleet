package engine

import (
	"path/filepath"
	"strings"
	"testing"
)

func inspectBody(t *testing.T, body string) []Problem {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "checkfleet.yml")
	writeVarFile(t, path, body)
	cfg, err := LoadConfig(path)
	if err != nil {
		cfg = nil
	}
	return Inspect(path, cfg)
}

func problemMentioning(problems []Problem, substr string) (Problem, bool) {
	for _, p := range problems {
		if strings.Contains(p.Message, substr) {
			return p, true
		}
	}
	return Problem{}, false
}

// The headline case: a misspelled module is silently dropped by YAML, so the
// module never runs and nothing used to say why.
func TestInspectSuggestsModuleTypo(t *testing.T) {
	problems := inspectBody(t, `checks:
  postgress:
    targets:
      - {name: pg, dsn: "host=db1"}
`)
	p, ok := problemMentioning(problems, `"postgress"`)
	if !ok {
		t.Fatalf("the misspelled module was not reported: %+v", problems)
	}
	if !strings.Contains(p.Suggestion, "postgres") {
		t.Errorf("suggestion = %q, want it to name postgres", p.Suggestion)
	}
	if !strings.Contains(p.Message, "ignored") {
		t.Errorf("the message should say the key is ignored: %q", p.Message)
	}
	if p.Advisory {
		t.Error("a typo is a config defect, not an advisory note")
	}
}

func TestInspectSuggestsTopLevelTypo(t *testing.T) {
	problems := inspectBody(t, "timeout_second: 5\nchecks:\n  certs:\n    targets: [example.com]\n")
	p, ok := problemMentioning(problems, "timeout_second")
	if !ok {
		t.Fatalf("the misspelled top-level key was not reported: %+v", problems)
	}
	if !strings.Contains(p.Suggestion, "timeout_seconds") {
		t.Errorf("suggestion = %q", p.Suggestion)
	}
}

// Regression: `include` is handled by the loader from the raw map, so it is not
// a Config field. Flagging it would fail a perfectly correct config.
func TestInspectAcceptsIncludeKey(t *testing.T) {
	dir := t.TempDir()
	inc := filepath.Join(dir, "extra.yml")
	writeVarFile(t, inc, "checks:\n  http:\n    targets: [{url: \"https://example.com/\"}]\n")
	path := filepath.Join(dir, "checkfleet.yml")
	writeVarFile(t, path, "include: extra.yml\nchecks:\n  certs:\n    targets: [example.com]\n")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range Inspect(path, cfg) {
		if strings.Contains(p.Message, "include") {
			t.Errorf("include must not be reported as unknown: %q", p.Message)
		}
	}
}

// A key that resembles nothing must get no suggestion. A confidently wrong
// "did you mean" is worse than none.
func TestInspectNoSuggestionForNonsense(t *testing.T) {
	problems := inspectBody(t, `checks:
  certs:
    targets: [example.com]
  zzzqqqwww:
    targets: [x]
`)
	p, ok := problemMentioning(problems, "zzzqqqwww")
	if !ok {
		t.Fatalf("the unknown key was not reported: %+v", problems)
	}
	if p.Suggestion != "" {
		t.Errorf("unexpected suggestion for a nonsense key: %q", p.Suggestion)
	}
}

// An unset variable is about the machine, not the config: reported, but it must
// not fail validate (which is documented for pre-commit hooks).
func TestInspectEnvIsAdvisory(t *testing.T) {
	problems := inspectBody(t, `checks:
  postgres:
    targets:
      - {name: pg, dsn: "postgres://u:${CF_SUGGEST_ABSENT}@db1:5432/x"}
`)
	p, ok := problemMentioning(problems, "CF_SUGGEST_ABSENT")
	if !ok {
		t.Fatalf("the unset variable was not reported: %+v", problems)
	}
	if !p.Advisory {
		t.Error("an unset variable must be advisory, or a pre-commit hook breaks without production secrets")
	}
	if !strings.Contains(p.Suggestion, "export CF_SUGGEST_ABSENT") {
		t.Errorf("suggestion should show how to fix it: %q", p.Suggestion)
	}
	if Blocking(problems) {
		t.Error("a config whose only problem is an unset variable must not block")
	}
}

func TestBlocking(t *testing.T) {
	if Blocking(nil) {
		t.Error("no problems must not block")
	}
	if Blocking([]Problem{{Message: "note", Advisory: true}}) {
		t.Error("advisory-only must not block")
	}
	if !Blocking([]Problem{{Message: "note", Advisory: true}, {Message: "real"}}) {
		t.Error("a real defect must block even alongside advisories")
	}
}

// ${...} inside a comment is prose, not a reference — checkfleet.example.yml
// has exactly that, and it used to be reported as a variable named "...".
func TestInspectIgnoresPlaceholderInComment(t *testing.T) {
	problems := inspectBody(t, `checks:
  certs:
    # the password comes from the environment via ${...} interpolation
    targets: [example.com]
`)
	for _, p := range problems {
		if strings.Contains(p.Message, "...") && strings.Contains(p.Message, "environment variable") {
			t.Errorf("a placeholder in a comment was read as a variable: %q", p.Message)
		}
	}
}

func TestInspectSuggestsInitForEmptyChecks(t *testing.T) {
	problems := inspectBody(t, "timeout_seconds: 5\nchecks: {}\n")
	p, ok := problemMentioning(problems, "no module configured")
	if !ok {
		t.Fatalf("an empty checks block should be reported: %+v", problems)
	}
	if !strings.Contains(p.Suggestion, "checkfleet init") {
		t.Errorf("suggestion should point at init: %q", p.Suggestion)
	}
}

// Inspect must work with a nil config (the load failed) and still report the
// raw-level problems, which are usually the cause.
func TestInspectWithNilConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "checkfleet.yml")
	writeVarFile(t, path, "timeout_second: 5\nchecks:\n  certs:\n    targets: [example.com]\n")

	problems := Inspect(path, nil)
	if _, ok := problemMentioning(problems, "timeout_second"); !ok {
		t.Errorf("raw-level problems must survive a nil config: %+v", problems)
	}
}

func TestProblemString(t *testing.T) {
	if got := (Problem{Message: "m"}).String(); got != "m" {
		t.Errorf("without a suggestion: %q", got)
	}
	if got := (Problem{Message: "m", Suggestion: "s"}).String(); got != "m → s" {
		t.Errorf("with a suggestion: %q", got)
	}
}

func TestEditDistance(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"", "abc", 3},
		{"abc", "", 3},
		{"postgres", "postgres", 0},
		{"postgress", "postgres", 1},
		{"htp", "http", 1},
		{"retires", "retries", 2}, // a transposition costs two edits
		{"kafka", "vault", 4},
	}
	for _, tt := range tests {
		if got := editDistance(tt.a, tt.b); got != tt.want {
			t.Errorf("editDistance(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestClosest(t *testing.T) {
	modules := moduleKeys()

	// Real typos resolve.
	for in, want := range map[string]string{
		"postgress": "postgres",
		"htp":       "http",
		"redi":      "redis",
		"kafk":      "kafka",
		"elastic":   "elasticsearch",
	} {
		if got := closest(in, modules); got != want {
			t.Errorf("closest(%q) = %q, want %q", in, got, want)
		}
	}

	// Nonsense resolves to nothing rather than to whatever is nearest.
	for _, in := range []string{"zzzqqqwww", "somethingentirelyelse"} {
		if got := closest(in, modules); got != "" {
			t.Errorf("closest(%q) = %q, want no suggestion", in, got)
		}
	}
}

// moduleKeys/topLevelKeys are derived from the structs, so they must actually
// contain what the config accepts.
func TestKeySetsAreDerivedFromTheStructs(t *testing.T) {
	modules := moduleKeys()
	if len(modules) < 29 {
		t.Errorf("got %d module keys, want at least 29: %v", len(modules), modules)
	}
	for _, want := range []string{"certs", "http", "postgres", "tls", "grpc"} {
		if closest(want, modules) != want {
			t.Errorf("module key %q missing from %v", want, modules)
		}
	}
	top := topLevelKeys()
	for _, want := range []string{"checks", "timeout_seconds", "retries", "labels", "include"} {
		found := false
		for _, k := range top {
			if k == want {
				found = true
			}
		}
		if !found {
			t.Errorf("top-level key %q missing from %v", want, top)
		}
	}
}
