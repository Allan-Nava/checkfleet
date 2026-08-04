package main

import (
	"strings"
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/moduledoc"
	"github.com/Allan-Nava/checkfleet/internal/registry"
)

func TestGeneratorIsDeterministic(t *testing.T) {
	// Same input, byte-identical output — otherwise the CI staleness gate
	// (CF-152) would fail on every run for no reason. Map iteration order is the
	// usual way this breaks, so run each generator a few times.
	for i := 0; i < 5; i++ {
		first, second := modulesDoc(), modulesDoc()
		if first != second {
			t.Fatalf("modulesDoc is not deterministic (run %d)", i)
		}
		first, second = schemaDoc(), schemaDoc()
		if first != second {
			t.Fatalf("schemaDoc is not deterministic (run %d)", i)
		}
	}
}

// TestEveryRegistryModuleIsCovered is the point of generating instead of
// writing by hand: a module added to the registry cannot be missing here.
func TestEveryRegistryModuleIsCovered(t *testing.T) {
	doc, schema := modulesDoc(), schemaDoc()
	all := registry.All(&engine.Config{})
	if len(all) == 0 {
		t.Fatal("registry returned no modules")
	}
	for _, name := range all {
		if !strings.Contains(doc, "## "+name+"\n") {
			t.Errorf("module %q has no section in modules.md", name)
		}
		if !strings.Contains(schema, "### `checks."+name+"`") {
			t.Errorf("module %q has no schema section", name)
		}
	}
}

func TestEveryModuleHasADescription(t *testing.T) {
	for _, name := range registry.All(&engine.Config{}) {
		if _, ok := moduledoc.Doc(name); !ok {
			t.Errorf("module %q is in the registry but has no moduledoc entry", name)
		}
	}
	if strings.Contains(modulesDoc(), "undocumented") {
		t.Error("modules.md contains an undocumented placeholder")
	}
}

// TestSchemaCarriesRealDefaults proves the defaults come from the code and not
// from a comment someone copied: these are set by applyDefaults, so if that
// function changes, this test tells you the reference changed with it.
func TestSchemaCarriesRealDefaults(t *testing.T) {
	schema := schemaDoc()
	for _, want := range []string{
		"| `timeout_seconds` | `int` | `30` |",
		"| `warn_days` | `int` | `30` |",
		"| `crit_days` | `int` | `7` |",
		"| `port` | `int` | `443` |",
	} {
		if !strings.Contains(schema, want) {
			t.Errorf("schema is missing the default row %q", want)
		}
	}
}

// TestSchemaExpandsTargetTypes: an opaque `CertsTarget` token is exactly what
// makes an assistant invent field names.
func TestSchemaExpandsTargetTypes(t *testing.T) {
	schema := schemaDoc()
	for _, want := range []string{"### `PostgresTarget`", "### `HTTPTarget`", "| `password_env` | `string` |"} {
		if !strings.Contains(schema, want) {
			t.Errorf("schema does not expand %q", want)
		}
	}
}

func TestGeneratedFilesCarryNoSecrets(t *testing.T) {
	both := strings.ToLower(modulesDoc() + schemaDoc())
	for _, pat := range []string{"password: ", "bearer ", "-----begin"} {
		if strings.Contains(both, pat) {
			t.Errorf("generated reference contains %q", strings.TrimSpace(pat))
		}
	}
}

func TestFullConfigEnablesEveryModule(t *testing.T) {
	cfg := fullConfig()
	for _, name := range registry.Names(cfg) {
		_ = name
	}
	if got, want := len(registry.Names(cfg)), len(registry.All(cfg)); got != want {
		t.Errorf("fullConfig enabled %d modules, want all %d — the schema would silently skip the rest", got, want)
	}
}
