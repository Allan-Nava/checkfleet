package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// skillPath is the skill source versioned with the code (CF-149). It is
// installed globally by the user, not repo-local: checkfleet is used *from*
// other repos and hosts, while this repo already has CLAUDE.md for development.
const skillPath = "../../skills/checkfleet/SKILL.md"

func readSkill(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("read skill: %v", err)
	}
	return string(raw)
}

// cliUsage runs the built binary with no arguments and returns the usage text
// it prints. That is the real surface, not a copy of it.
func cliUsage(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "checkfleet")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	out, _ := exec.Command(bin).CombinedOutput() // exits 64, usage on stderr
	return string(out)
}

func TestSkillFrontMatterIsWellFormed(t *testing.T) {
	body := readSkill(t)
	if !strings.HasPrefix(body, "---\n") {
		t.Fatal("skill must open with a YAML front-matter fence")
	}
	end := strings.Index(body[4:], "\n---\n")
	if end < 0 {
		t.Fatal("front matter is not closed by a --- fence")
	}
	fm := body[4 : 4+end]
	for _, key := range []string{"name:", "description:"} {
		if !strings.Contains(fm, key) {
			t.Errorf("front matter is missing %q", key)
		}
	}
	if !strings.Contains(fm, "name: checkfleet") {
		t.Error(`name must be exactly "checkfleet" — it is the invocation name`)
	}
	// The description is what an agent matches on to decide whether the skill is
	// relevant. A paraphrase of the name ("the checkfleet skill") never matches a
	// real question, so require it to name concrete triggers.
	for _, trigger := range []string{"certificate", "postgres", "redis"} {
		if !strings.Contains(strings.ToLower(fm), trigger) {
			t.Errorf("description does not mention %q — it needs real triggers, not a paraphrase of the name", trigger)
		}
	}
}

// maxSkillBytes caps the always-loaded body. Context budget is the scarce
// resource: anything longer belongs in references/, loaded on demand.
const maxSkillBytes = 6000

func TestSkillStaysSmall(t *testing.T) {
	if n := len(readSkill(t)); n > maxSkillBytes {
		t.Errorf("SKILL.md is %d bytes, over the %d cap — move detail into references/", n, maxSkillBytes)
	}
}

var (
	subcommandRe = regexp.MustCompile(`checkfleet ([a-z][a-z-]+)`)
	flagRe       = regexp.MustCompile(`--([a-z][a-z-]+)`)
	fenceRe      = regexp.MustCompile("(?s)```[a-z]*\n(.*?)```")
	inlineRe     = regexp.MustCompile("`([^`\n]+)`")
)

// codeOf returns only the parts of the document an agent would copy and run:
// fenced blocks and inline-code spans. Prose is excluded on purpose — a
// sentence like "can checkfleet see X?" is not a command, and treating it as
// one would push the gate below toward a growing exception list instead of a
// real check.
func codeOf(body string) string {
	var b strings.Builder
	for _, m := range fenceRe.FindAllStringSubmatch(body, -1) {
		b.WriteString(m[1])
		b.WriteByte('\n')
	}
	for _, m := range inlineRe.FindAllStringSubmatch(body, -1) {
		b.WriteString(m[1])
		b.WriteByte('\n')
	}
	return b.String()
}

// TestSkillCitesRealCommands is the anti-fiction gate: every subcommand and flag
// the skill shows as runnable must exist in the CLI. A skill that confidently
// cites a flag that was renamed is worse than no skill — the agent will keep
// trying it and blame the environment.
func TestSkillCitesRealCommands(t *testing.T) {
	code, usage := codeOf(readSkill(t)), cliUsage(t)

	var bad []string
	for _, m := range subcommandRe.FindAllStringSubmatch(code, -1) {
		if !strings.Contains(usage, "checkfleet "+m[1]) {
			bad = append(bad, "subcommand "+m[1])
		}
	}
	for _, m := range flagRe.FindAllStringSubmatch(code, -1) {
		if !strings.Contains(usage, "--"+m[1]) {
			bad = append(bad, "flag --"+m[1])
		}
	}
	sort.Strings(bad)
	if len(bad) > 0 {
		t.Errorf("the skill cites things the CLI does not have: %s", strings.Join(uniq(bad), ", "))
	}
}

// TestSkillGateCatchesAFakeFlag proves the gate above can fail. A validator
// nobody has seen reject anything is not a validator.
func TestSkillGateCatchesAFakeFlag(t *testing.T) {
	code := codeOf("```bash\ncheckfleet check all --no-such-flag\n```")
	if !strings.Contains(code, "--no-such-flag") {
		t.Fatal("codeOf dropped a fenced command")
	}
	if strings.Contains(cliUsage(t), "--no-such-flag") {
		t.Fatal("the CLI unexpectedly has --no-such-flag")
	}
}

func uniq(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// TestSkillStatesTheTwoSemantics guards the two rules that decide whether the
// output is read correctly at all. They are the reason this skill exists.
func TestSkillStatesTheTwoSemantics(t *testing.T) {
	body := readSkill(t)
	if !strings.Contains(body, "--exit-on") {
		t.Error("the skill must show --exit-on: without it, exit 0 reads as healthy")
	}
	if !strings.Contains(body, "ERROR") || !strings.Contains(body, "BAD") {
		t.Error("the skill must distinguish ERROR from BAD")
	}
	if !strings.Contains(body, "worst") {
		t.Error("the skill must point at the JSON worst field instead of grepping text")
	}
}

func TestSkillCarriesNoSecrets(t *testing.T) {
	body := strings.ToLower(readSkill(t))
	// Placeholders naming an env var are fine; a literal assignment is not.
	for _, pat := range []string{"password: ", "token: ", "secret: ", "api_key: "} {
		if strings.Contains(body, pat) {
			t.Errorf("skill contains what looks like a literal %q", strings.TrimSpace(pat))
		}
	}
}
