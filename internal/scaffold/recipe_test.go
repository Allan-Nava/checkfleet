package scaffold

import (
	"strings"
	"testing"
)

// Every recipe must name only modules that can actually be scaffolded — a
// recipe pointing at a module with no snippet would fail at the moment a user
// picks it, which is the worst place to find out.
func TestRecipesOnlyNameScaffoldableModules(t *testing.T) {
	supported := map[string]bool{}
	for _, m := range Supported() {
		supported[m] = true
	}
	for _, name := range Recipes() {
		r, ok := RecipeByName(name)
		if !ok {
			t.Fatalf("Recipes() returned %q but RecipeByName does not know it", name)
		}
		if len(r.Modules) == 0 {
			t.Errorf("recipe %q has no modules", name)
		}
		if r.Summary == "" {
			t.Errorf("recipe %q has no summary", name)
		}
		for _, m := range r.Modules {
			if !supported[m] {
				t.Errorf("recipe %q names module %q, which init cannot scaffold", name, m)
			}
		}
	}
}

// The contract that matters: whatever a recipe produces must be a config
// checkfleet accepts, so `init --recipe x && check all` works with no editing.
func TestEveryRecipeProducesAValidConfig(t *testing.T) {
	for _, name := range Recipes() {
		body, err := ConfigForRecipe(name)
		if err != nil {
			t.Errorf("recipe %q: %v", name, err)
			continue
		}
		if !strings.Contains(body, "# Recipe: "+name) {
			t.Errorf("recipe %q: the header should name it:\n%s", name, body)
		}
		if !strings.Contains(body, "checks:") {
			t.Errorf("recipe %q produced no checks block:\n%s", name, body)
		}
		// Parsed and validated by the engine in scaffold_validate_test.go, which
		// lives in cmd/ to avoid an import cycle; here we assert the shape.
		for _, m := range mustRecipe(t, name).Modules {
			if !strings.Contains(body, "  "+m+":") {
				t.Errorf("recipe %q is missing its %q block:\n%s", name, m, body)
			}
		}
	}
}

func TestRecipeUnknownName(t *testing.T) {
	if _, err := ConfigForRecipe("nope"); err == nil {
		t.Error("an unknown recipe should be an error")
	} else if !strings.Contains(err.Error(), "available:") {
		t.Errorf("the error should list the recipes: %v", err)
	}
}

func TestRecipeNameIsCaseInsensitive(t *testing.T) {
	if _, ok := RecipeByName("WEB"); !ok {
		t.Error("recipe lookup should be case-insensitive")
	}
	if _, ok := RecipeByName("  web "); !ok {
		t.Error("recipe lookup should tolerate surrounding space")
	}
}

func mustRecipe(t *testing.T, name string) Recipe {
	t.Helper()
	r, ok := RecipeByName(name)
	if !ok {
		t.Fatalf("unknown recipe %q", name)
	}
	return r
}
