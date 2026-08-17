package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/moduledoc"
	"github.com/Allan-Nava/checkfleet/internal/registry"
)

// TestEveryModuleAnswersThePermissionsQuestion is the gate CF-182 asks for: a
// module must either produce grant statements or say plainly that it needs no
// credential. Silence is the failure mode — a module that answers neither looks
// like it needs nothing, and that is how a privilege requirement ships
// undocumented.
func TestEveryModuleAnswersThePermissionsQuestion(t *testing.T) {
	all := registry.All(&engine.Config{})
	if len(all) == 0 {
		t.Fatal("registry returned no modules")
	}
	for _, name := range all {
		p, ok := moduledoc.Perms(name)
		if !ok {
			t.Errorf("%s: no permissions entry — `checkfleet perms` would say nothing about it", name)
			continue
		}
		if !p.Unauthenticated && len(p.Statements) == 0 && !p.NeedsJudgement {
			t.Errorf("%s: needs a credential but offers no statements and claims no judgement call", name)
		}
	}
}

func TestPermsTextCoversASingleModule(t *testing.T) {
	out := permsText([]string{"postgres"})
	if !strings.Contains(out, "GRANT pg_monitor TO checkfleet;") {
		t.Errorf("missing the grant:\n%s", out)
	}
	if !strings.Contains(out, "Not needed:") {
		t.Error("the half that gets the ticket approved is missing")
	}
}

// TestPermsGroupsTheCredentialFreeModules — seventeen of twenty-nine need
// nothing, and listing them one by one buries the ones that do.
func TestPermsGroupsTheCredentialFreeModules(t *testing.T) {
	out := permsText([]string{"certs", "dns", "postgres"})
	if !strings.Contains(out, "No credential needed (2): certs, dns") {
		t.Errorf("credential-free modules should be summarised on one line:\n%s", out)
	}
	if !strings.Contains(out, "postgres") {
		t.Error("the module that does need a grant must still be detailed")
	}
}

func TestPermsSaysSoWhenNothingIsNeeded(t *testing.T) {
	out := permsText([]string{"certs", "tcp"})
	if !strings.Contains(out, "Nothing to grant") {
		t.Errorf("a fleet that needs no grant should be told so:\n%s", out)
	}
}

// TestPermsPrintsNoCredentials — this output gets pasted into tickets.
func TestPermsPrintsNoCredentials(t *testing.T) {
	all := registry.All(&engine.Config{})
	out := permsText(all) + permsMarkdown(all)
	for _, bad := range []string{"password123", "hunter2", "changeme"} {
		if strings.Contains(strings.ToLower(out), bad) {
			t.Errorf("output contains a plausible credential %q", bad)
		}
	}
	// Every password-setting statement must carry the placeholder, never a value
	// that could be pasted as-is into production.
	for _, line := range strings.Split(out, "\n") {
		low := strings.ToLower(line)
		if !strings.Contains(low, "password") && !strings.Contains(low, "pwd") {
			continue
		}
		if !strings.Contains(line, "<from your secret store>") && !strings.Contains(low, "not needed") {
			t.Errorf("a credential line without the placeholder: %q", line)
		}
	}
}

func TestPermsJSONShape(t *testing.T) {
	// permsJSON writes to stdout, so exercise the entry shape it builds.
	p, _ := moduledoc.Perms("redis")
	raw, err := json.Marshal(permEntry{
		Module: "redis", Summary: p.Summary, Statements: p.Statements,
		NotNeeded: p.NotNeeded, Unauthenticated: p.Unauthenticated,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"module"`, `"summary"`, `"statements"`, `"not_needed"`} {
		if !strings.Contains(string(raw), key) {
			t.Errorf("missing %s in %s", key, raw)
		}
	}
}

func TestPermModulesResolution(t *testing.T) {
	// A named module wins over everything.
	got, err := permModules("redis", "", "")
	if err != nil || len(got) != 1 || got[0] != "redis" {
		t.Errorf("named module = %v, %v", got, err)
	}
	// An unknown one is a systemic error, not an empty report.
	if _, err := permModules("nope", "", ""); err == nil {
		t.Error("an unknown module must error")
	}
	// No config: every module.
	all, err := permModules("", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != len(registry.All(&engine.Config{})) {
		t.Errorf("without a config, want every module, got %d", len(all))
	}
}

// TestPermsWithAConfigCoversOnlyWhatIsConfigured: a fleet running two modules
// should not hand its DBA the grants for twenty-nine.
func TestPermsWithAConfigCoversOnlyWhatIsConfigured(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "checkfleet.yml")
	body := "checks:\n  certs:\n    targets: [example.com]\n  redis:\n    targets: [127.0.0.1]\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := permModules("", path, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "certs" || got[1] != "redis" {
		t.Errorf("configured modules = %v, want [certs redis]", got)
	}
}

func TestPermsRejectsAnUnknownFormat(t *testing.T) {
	if err := runPerms([]string{"--output", "yaml"}); err == nil {
		t.Error("an unknown format must be a systemic error")
	}
}

func TestWrapAtKeepsWordsWhole(t *testing.T) {
	got := wrapAt("alpha beta gamma delta", 11, "  ")
	if strings.Contains(got, "alp\n") {
		t.Errorf("wrapping split a word: %q", got)
	}
	if !strings.Contains(got, "\n  ") {
		t.Errorf("expected an indented continuation line: %q", got)
	}
	if wrapAt("", 10, "") != "" {
		t.Error("empty input should wrap to empty")
	}
}

// TestPermsAcceptsTheModuleBeforeTheFlags is a regression guard. Go's flag
// package stops parsing at the first non-flag argument, so `perms redis
// --output json` silently ignored the format and printed text — and the natural
// way to type the command is exactly that order.
func TestPermsAcceptsTheModuleBeforeTheFlags(t *testing.T) {
	for _, args := range [][]string{
		{"redis", "--output", "markdown"},
		{"--output", "markdown", "redis"},
	} {
		out := captureStdout(t, func() {
			if err := runPerms(args); err != nil {
				t.Fatalf("%v: %v", args, err)
			}
		})
		if !strings.Contains(out, "# checkfleet — required permissions") {
			t.Errorf("%v: the markdown format was ignored:\n%s", args, out)
		}
		if !strings.Contains(out, "## redis") {
			t.Errorf("%v: the named module was ignored", args)
		}
	}
}

// captureStdout runs f with os.Stdout redirected and returns what it wrote.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		var sb strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			sb.Write(buf[:n])
			if err != nil {
				break
			}
		}
		done <- sb.String()
	}()
	f()
	os.Stdout = orig
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return <-done
}
