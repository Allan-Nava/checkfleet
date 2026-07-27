package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The composite action at the repo root is a published interface: a typo in it
// only shows up in someone else's pipeline. These tests keep it honest.

type actionFile struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Inputs      map[string]struct {
		Description string  `yaml:"description"`
		Required    bool    `yaml:"required"`
		Default     *string `yaml:"default"`
	} `yaml:"inputs"`
	Outputs map[string]struct {
		Description string `yaml:"description"`
		Value       string `yaml:"value"`
	} `yaml:"outputs"`
	Runs struct {
		Using string `yaml:"using"`
		Steps []struct {
			Name  string            `yaml:"name"`
			ID    string            `yaml:"id"`
			Shell string            `yaml:"shell"`
			Env   map[string]string `yaml:"env"`
			Run   string            `yaml:"run"`
		} `yaml:"steps"`
	} `yaml:"runs"`
}

func loadAction(t *testing.T) (actionFile, string) {
	t.Helper()
	path := filepath.Join("..", "..", "action.yml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading action.yml: %v", err)
	}
	var a actionFile
	if err := yaml.Unmarshal(raw, &a); err != nil {
		t.Fatalf("action.yml is not valid YAML: %v", err)
	}
	return a, string(raw)
}

func TestActionIsWellFormed(t *testing.T) {
	a, _ := loadAction(t)

	if a.Name == "" || a.Description == "" {
		t.Error("action.yml needs both a name and a description")
	}
	if a.Runs.Using != "composite" {
		t.Errorf("runs.using = %q, want composite", a.Runs.Using)
	}
	if len(a.Runs.Steps) == 0 {
		t.Fatal("the action has no steps")
	}
	for i, s := range a.Runs.Steps {
		// A composite step without a shell is a hard error at run time.
		if s.Run != "" && s.Shell == "" {
			t.Errorf("step %d (%q) runs a script with no shell:", i, s.Name)
		}
	}
}

func TestActionInputsAreDocumented(t *testing.T) {
	a, _ := loadAction(t)
	if len(a.Inputs) == 0 {
		t.Fatal("the action declares no inputs")
	}
	for name, in := range a.Inputs {
		if in.Description == "" {
			t.Errorf("input %q has no description", name)
		}
		// Everything is optional by design: `uses: Allan-Nava/checkfleet@v1`
		// with no `with:` block must be a working default.
		if in.Required {
			t.Errorf("input %q is required; the action should work with no `with:` block", name)
		}
		if in.Default == nil {
			t.Errorf("input %q has no default", name)
		}
	}
}

// An input that is declared but never reaches the script is silently ignored —
// the user sets it, nothing happens, and nothing says so.
func TestActionInputsAreAllUsed(t *testing.T) {
	a, raw := loadAction(t)
	for name := range a.Inputs {
		ref := "inputs." + name
		if !strings.Contains(raw, ref) {
			t.Errorf("input %q is declared but never referenced as ${{ %s }}", name, ref)
		}
	}
}

// Inputs must reach bash through env, not through ${{ }} interpolated into the
// script body, where an input containing shell metacharacters would execute.
func TestActionDoesNotInterpolateInputsIntoScripts(t *testing.T) {
	a, _ := loadAction(t)
	for _, s := range a.Runs.Steps {
		if strings.Contains(s.Run, "${{") {
			t.Errorf("step %q interpolates an expression into its script; pass it via env instead:\n%s",
				s.Name, s.Run)
		}
	}
}

func TestActionOutputsPointAtRealSteps(t *testing.T) {
	a, _ := loadAction(t)
	ids := map[string]bool{}
	for _, s := range a.Runs.Steps {
		if s.ID != "" {
			ids[s.ID] = true
		}
	}
	for name, out := range a.Outputs {
		if out.Description == "" {
			t.Errorf("output %q has no description", name)
		}
		// value looks like ${{ steps.<id>.outputs.<key> }}
		_, rest, ok := strings.Cut(out.Value, "steps.")
		if !ok {
			continue
		}
		id, _, _ := strings.Cut(rest, ".")
		if !ids[id] {
			t.Errorf("output %q references step id %q, which no step declares", name, id)
		}
	}
}

// The flags the action passes must exist in the CLI. This is the seam that
// breaks quietly: rename a flag and only a downstream pipeline finds out.
func TestActionUsesRealFlags(t *testing.T) {
	_, raw := loadAction(t)
	bin := buildCLI(t)

	for _, flag := range []string{
		"--config", "--output", "--exit-on", "--exit-code", "--stack",
		"--out-file", "--baseline", "--fail-on-new", "--min-severity", "--target",
	} {
		if !strings.Contains(raw, flag) {
			continue // not used by the action; nothing to verify
		}
		out, code := runCLI(t, bin, "check", "all", "--config", "/nonexistent.yml", flag+"=x")
		// An unknown flag makes the flag package print "flag provided but not
		// defined" and exit 2; a known one gets as far as the missing config.
		if strings.Contains(out, "not defined") {
			t.Errorf("action.yml passes %s, which the CLI does not define (exit %d):\n%s", flag, code, out)
		}
	}
}
