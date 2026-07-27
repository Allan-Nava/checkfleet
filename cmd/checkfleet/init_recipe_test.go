package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The end-to-end promise of a recipe: init it, then check it, with no editing.
func TestInitRecipeProducesAConfigValidateAccepts(t *testing.T) {
	bin := buildCLI(t)
	for _, recipe := range []string{"web", "db", "edge", "media"} {
		t.Run(recipe, func(t *testing.T) {
			cfg := filepath.Join(t.TempDir(), "checkfleet.yml")
			if out, code := runCLI(t, bin, "init", "--recipe", recipe, "--config", cfg); code != 0 {
				t.Fatalf("init --recipe %s exited %d:\n%s", recipe, code, out)
			}
			body, err := os.ReadFile(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), "# Recipe: "+recipe) {
				t.Errorf("the file should name its recipe:\n%s", body)
			}
			if out, code := runCLI(t, bin, "validate", "--config", cfg); code != 0 {
				t.Fatalf("the generated config must validate, got %d:\n%s", code, out)
			}
		})
	}
}

func TestInitFromInventory(t *testing.T) {
	bin := buildCLI(t)
	dir := t.TempDir()
	inv := filepath.Join(dir, "hosts.ini")
	body := "[web]\nweb1 ansible_host=10.0.0.5\nweb2\n\n[db]\ndb1\n"
	if err := os.WriteFile(inv, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(dir, "checkfleet.yml")

	out, code := runCLI(t, bin, "init", "--from-inventory", inv, "--modules", "certs,http", "--config", cfg)
	if code != 0 {
		t.Fatalf("exited %d:\n%s", code, out)
	}
	got, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	// Every host, with ansible_host preferred where set.
	for _, want := range []string{"10.0.0.5:443", "web2:443", "db1:443"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
	if out, code := runCLI(t, bin, "validate", "--config", cfg); code != 0 {
		t.Fatalf("the generated config must validate, got %d:\n%s", code, out)
	}

	// --group narrows it.
	cfg2 := filepath.Join(dir, "web.yml")
	if out, code := runCLI(t, bin, "init", "--from-inventory", inv, "--group", "web", "--config", cfg2); code != 0 {
		t.Fatalf("--group exited %d:\n%s", code, out)
	}
	got2, err := os.ReadFile(cfg2)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got2), "db1") {
		t.Errorf("--group web must exclude the db group:\n%s", got2)
	}
}

func TestInitFlagCombinations(t *testing.T) {
	bin := buildCLI(t)
	dir := t.TempDir()
	inv := filepath.Join(dir, "hosts.ini")
	if err := os.WriteFile(inv, []byte("[web]\nweb1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(dir, "checkfleet.yml")

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"recipe with inventory", []string{"init", "--recipe", "web", "--from-inventory", inv, "--config", cfg}, "use one"},
		{"recipe with modules", []string{"init", "--recipe", "web", "--modules", "certs", "--config", cfg}, "drop --modules"},
		{"group without inventory", []string{"init", "--group", "web", "--config", cfg}, "--from-inventory"},
		{"unknown recipe", []string{"init", "--recipe", "nope", "--config", cfg}, "available:"},
		{"missing inventory", []string{"init", "--from-inventory", filepath.Join(dir, "nope.ini"), "--config", cfg}, "inventory"},
		{"empty group", []string{"init", "--from-inventory", inv, "--group", "nosuch", "--config", cfg}, "no hosts in group"},
		{"underivable module", []string{"init", "--from-inventory", inv, "--modules", "postgres", "--config", cfg}, "cannot be derived"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, code := runCLI(t, bin, tt.args...)
			if code != 1 {
				t.Fatalf("want exit 1, got %d:\n%s", code, out)
			}
			if !strings.Contains(out, tt.want) {
				t.Errorf("error should mention %q:\n%s", tt.want, out)
			}
		})
	}
}

func TestInitListShowsRecipes(t *testing.T) {
	bin := buildCLI(t)
	out, code := runCLI(t, bin, "init", "--list")
	if code != 0 {
		t.Fatalf("exited %d:\n%s", code, out)
	}
	for _, want := range []string{"Recipes", "web", "db", "edge", "media", "from an inventory"} {
		if !strings.Contains(out, want) {
			t.Errorf("--list should mention %q:\n%s", want, out)
		}
	}
}
