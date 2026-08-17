package main

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Allan-Nava/checkfleet/skills"
)

func TestSkillInstallWritesTheTree(t *testing.T) {
	dir := t.TempDir()
	if err := runSkill([]string{"install", "--dir", dir}); err != nil {
		t.Fatalf("install: %v", err)
	}
	for _, rel := range []string{
		"checkfleet/SKILL.md",
		"checkfleet/references/modules.md",
		"checkfleet/references/config-schema.md",
		"checkfleet/references/permissions.md",
	} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("missing %s: %v", rel, err)
		}
	}
}

// TestSkillInstallIsIdempotent: re-running after an upgrade must land on the
// same tree, not accumulate or fail on existing files.
func TestSkillInstallIsIdempotent(t *testing.T) {
	// Compare the tree after one install with the tree after two, rather than
	// against a fixed count: adding a reference file is a normal thing to do and
	// should not make this test fail for the wrong reason.
	dir := t.TempDir()
	if err := runSkill([]string{"install", "--dir", dir}); err != nil {
		t.Fatalf("first install: %v", err)
	}
	first := treeOf(t, dir)
	if len(first) < 2 {
		t.Fatalf("the install wrote %d files; the walk is broken, not the code", len(first))
	}
	if err := runSkill([]string{"install", "--dir", dir}); err != nil {
		t.Fatalf("second install: %v", err)
	}
	second := treeOf(t, dir)
	if strings.Join(first, ",") != strings.Join(second, ",") {
		t.Errorf("re-installing changed the tree:\n first: %v\nsecond: %v", first, second)
	}
}

// treeOf lists the files under dir, relative and sorted.
func treeOf(t *testing.T, dir string) []string {
	t.Helper()
	var got []string
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			rel, _ := filepath.Rel(dir, p)
			got = append(got, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	return got
}

// TestSkillInstallOverwritesAnOldVersion — an upgrade must replace the shipped
// files, otherwise the binary and its skill disagree about which flags exist.
func TestSkillInstallOverwritesAnOldVersion(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, "checkfleet", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("old version --gone-flag\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runSkill([]string{"install", "--dir", dir}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(stale)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "--gone-flag") {
		t.Error("install did not overwrite the stale skill")
	}
	if !strings.Contains(string(body), "name: checkfleet") {
		t.Error("installed file is not the embedded skill")
	}
}

func TestSkillPrintEmitsTheSkill(t *testing.T) {
	body, err := skills.FS.ReadFile(skills.Root + "/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(body), "---\n") {
		t.Error("embedded skill does not start with front matter")
	}
}

// TestEmbeddedSkillMatchesTheSource is the reason to embed rather than copy:
// the bytes in the binary must be the bytes in the repo.
func TestEmbeddedSkillMatchesTheSource(t *testing.T) {
	for _, rel := range []string{"SKILL.md", "references/modules.md", "references/config-schema.md", "references/permissions.md"} {
		onDisk, err := os.ReadFile(filepath.Join("..", "..", "skills", "checkfleet", rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		embedded, err := skills.FS.ReadFile(skills.Root + "/" + rel)
		if err != nil {
			t.Fatalf("embedded %s: %v", rel, err)
		}
		if string(onDisk) != string(embedded) {
			t.Errorf("%s: embedded copy differs from the source on disk", rel)
		}
	}
}

func TestSkillRejectsAnUnknownSubcommand(t *testing.T) {
	if err := runSkill([]string{"frobnicate"}); err == nil {
		t.Error("want an error for an unknown subcommand")
	}
	if err := runSkill(nil); err == nil {
		t.Error("want a usage error with no subcommand")
	}
}

// TestCompletionListsEverySubcommand keeps the completion script honest against
// the dispatch in main.go. It had drifted: init, alert, doctor and targets
// shipped without ever being completable, and `skill` would have joined them.
func TestCompletionListsEverySubcommand(t *testing.T) {
	script, err := completionScript("bash")
	if err != nil {
		t.Fatal(err)
	}
	usage := captureUsage(t)
	checked := 0
	for _, line := range strings.Split(usage, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 || fields[0] != "checkfleet" {
			continue
		}
		cmd := fields[1]
		if strings.HasPrefix(cmd, "<") || strings.HasPrefix(cmd, "-") {
			continue
		}
		checked++
		if !strings.Contains(script, cmd) {
			t.Errorf("subcommand %q is in the usage but not completable", cmd)
		}
	}
	// Without this the loop could silently examine nothing and always pass — the
	// failure mode of every "parse the docs" test.
	if checked < 10 {
		t.Fatalf("only parsed %d subcommands out of the usage; the parser is broken, not the script", checked)
	}
}

// captureUsage reads the usage text by redirecting os.Stderr around usage().
func captureUsage(t *testing.T) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w
	usage()
	os.Stderr = orig
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	if _, err := io.Copy(&b, r); err != nil {
		t.Fatal(err)
	}
	return b.String()
}
