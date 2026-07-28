package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// confd creates dir/name as a drop-in directory and returns the directory path.
func confd(t *testing.T, dir, name string) string {
	t.Helper()
	sub := filepath.Join(dir, name)
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	return sub
}

func TestUnknownKeysNamesModuleTypo(t *testing.T) {
	cfg := writeFile(t, t.TempDir(), "checkfleet.yml",
		"checks:\n  postgress:\n    targets:\n      - {name: pg, dsn: \"host=db1\"}\n")

	problems := UnknownKeys(cfg)
	if len(problems) != 1 {
		t.Fatalf("want one problem, got %+v", problems)
	}
	if !strings.Contains(problems[0].Message, `"postgress"`) {
		t.Errorf("message must name the key: %q", problems[0].Message)
	}
	if !strings.Contains(problems[0].Suggestion, "postgres") {
		t.Errorf("suggestion must point at the real module: %q", problems[0].Suggestion)
	}
}

func TestUnknownKeysCleanConfigIsSilent(t *testing.T) {
	cfg := writeFile(t, t.TempDir(), "checkfleet.yml",
		"timeout_seconds: 5\nchecks:\n  tcp:\n    targets:\n      - {name: db, address: db1:5432}\n")

	if problems := UnknownKeys(cfg); len(problems) != 0 {
		t.Errorf("a valid config must produce no notice, got %+v", problems)
	}
}

// A typo in an included file is the same bug, and harder to spot — the main
// config looks fine. UnknownKeys follows the include chain (CF-115) so it does
// not stop at the file it was handed.
func TestUnknownKeysFollowsIncludes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, confd(t, dir, "conf.d"), "web.yml", "checks:\n  htp:\n    targets:\n      - {name: a, url: \"https://a.example\"}\n")
	cfg := writeFile(t, dir, "checkfleet.yml", "include: [conf.d]\nchecks:\n  tcp:\n    targets:\n      - {name: db, address: db1:5432}\n")

	problems := UnknownKeys(cfg)
	if len(problems) != 1 {
		t.Fatalf("want the typo from the included file, got %+v", problems)
	}
	if !strings.Contains(problems[0].Message, "web.yml") {
		t.Errorf("message must name the file the key is in: %q", problems[0].Message)
	}
	if !strings.Contains(problems[0].Suggestion, "http") {
		t.Errorf("suggestion must point at the real module: %q", problems[0].Suggestion)
	}
}

func TestUnknownKeysIgnoresIncludeItself(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "extra.yml", "checks:\n  tcp:\n    targets:\n      - {name: db, address: db1:5432}\n")
	cfg := writeFile(t, dir, "checkfleet.yml", "include: [extra.yml]\n")

	if problems := UnknownKeys(cfg); len(problems) != 0 {
		t.Errorf("`include` is a documented key, not an unknown one: %+v", problems)
	}
}

func TestUnknownKeysMissingFile(t *testing.T) {
	if problems := UnknownKeys(filepath.Join(t.TempDir(), "nope.yml")); len(problems) != 0 {
		t.Errorf("a missing file is the loader's error to report, got %+v", problems)
	}
}

// An include cycle must not hang or recurse forever — the loader reports it, we
// just have to not make it worse.
func TestUnknownKeysIncludeCycle(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.yml", "include: [b.yml]\n")
	writeFile(t, dir, "b.yml", "include: [a.yml]\nchecks:\n  postgress: {}\n")

	problems := UnknownKeys(filepath.Join(dir, "a.yml"))
	if len(problems) != 1 || !strings.Contains(problems[0].Message, "postgress") {
		t.Errorf("want the typo found once despite the cycle, got %+v", problems)
	}
}
