package scaffold

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/inventory"
)

// loadAndValidate writes body to a temp file and puts it through the real
// loader and validator — the only check that means anything for a scaffold:
// `init` must produce a config the tool actually accepts.
func loadAndValidate(t *testing.T, label, body string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "checkfleet.yml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := engine.LoadConfig(path)
	if err != nil {
		t.Errorf("%s: generated config does not load: %v\n%s", label, err, body)
		return
	}
	if problems := engine.Validate(cfg); len(problems) > 0 {
		t.Errorf("%s: generated config does not validate: %v\n%s", label, problems, body)
	}
	// Inspect catches what Validate can't: a key the scaffold misspelled would
	// be silently dropped, producing a config that checks nothing.
	for _, p := range engine.Inspect(path, cfg) {
		if !p.Advisory {
			t.Errorf("%s: %s\n%s", label, p, body)
		}
	}
}

func TestRecipesProduceLoadableValidConfigs(t *testing.T) {
	for _, name := range Recipes() {
		body, err := ConfigForRecipe(name)
		if err != nil {
			t.Errorf("recipe %q: %v", name, err)
			continue
		}
		loadAndValidate(t, "recipe "+name, body)
	}
}

func TestEveryModuleSnippetProducesAValidConfig(t *testing.T) {
	for _, m := range Supported() {
		body, err := Config([]string{m})
		if err != nil {
			t.Errorf("module %q: %v", m, err)
			continue
		}
		loadAndValidate(t, "module "+m, body)
	}
}

func TestInventoryScaffoldProducesValidConfigs(t *testing.T) {
	hosts := []inventory.Host{
		{Name: "web1", Address: "10.0.0.5", Group: "web"},
		{Name: "web2", Address: "web2.example.com", Group: "web"},
	}
	// Each module on its own, then all of them together.
	for _, m := range InventoryModules() {
		body, err := FromInventory(hosts, []string{m})
		if err != nil {
			t.Errorf("module %q: %v", m, err)
			continue
		}
		loadAndValidate(t, "inventory "+m, body)
	}
	body, err := FromInventory(hosts, InventoryModules())
	if err != nil {
		t.Fatal(err)
	}
	loadAndValidate(t, "inventory all", body)
}
