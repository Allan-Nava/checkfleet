package main

import (
	"reflect"
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

// TestInlinedKeysAreDocumented is a regression guard for CF-183. ClientTLS is
// spliced into six module configs with `yaml:",inline"`, and the generator used
// to skip such fields because their own yaml key is empty — so client_cert,
// client_key and ca_cert shipped absent from the reference an assistant reads
// to avoid inventing key names. Exactly the drift this file exists to prevent.
func TestTopLevelListTypesAreDocumented(t *testing.T) {
	// alert_routes, depends_on, maintenance and runbooks are lists of structs
	// hanging off the root. Their keys were missing for the same reason the
	// inlined ones were — nothing walked into them — and an assistant reading
	// this reference would have invented the names.
	schema := schemaDoc()
	for _, want := range []string{
		"### `AlertRoute`", "`renotify_after`", "`key_env`",
		"### `DependsRule`", "`on_check`", "`same_host`",
		"### `MaintenanceWindow`", "### `RunbookRule`",
	} {
		if !strings.Contains(schema, want) {
			t.Errorf("%s is missing from the config schema", want)
		}
	}
}

func TestInlinedKeysAreDocumented(t *testing.T) {
	schema := schemaDoc()
	for _, key := range []string{"`client_cert`", "`client_key`", "`ca_cert`"} {
		if !strings.Contains(schema, key) {
			t.Errorf("%s is missing from the config schema", key)
		}
	}
	// Present under every module that takes it, not just the first.
	for _, mod := range []string{"checks.http", "checks.grpc", "checks.tcp", "checks.smtp", "checks.elasticsearch", "checks.kafka"} {
		i := strings.Index(schema, "### `"+mod+"`")
		if i < 0 {
			t.Errorf("%s section missing", mod)
			continue
		}
		rest := schema[i:]
		if j := strings.Index(rest[5:], "### `"); j >= 0 {
			rest = rest[:j+5]
		}
		if !strings.Contains(rest, "client_cert") {
			t.Errorf("%s does not document client_cert", mod)
		}
	}
}

// This generator has now dropped keys from the schema four separate times, each
// found by hand and fixed with a one-off regression test: inline fields, list
// types at the top level, nested keys checked only one level deep, and finally
// the types referenced from an inline struct (ConsulService and SRVLookup were
// named in every table and documented nowhere).
//
// So this test stops testing the symptoms. It walks every struct type reachable
// from engine.Config the way yaml.v3 will, and asserts that each one either has
// a section of its own or is inline (spliced into its parent's table). A key
// missing from this document is not cosmetic: the agent skill reads it as the
// definition of what checkfleet.yml accepts, so an undocumented option is an
// option that effectively does not exist.
func TestSchemaDocumentsEveryReachableType(t *testing.T) {
	doc := schemaDoc()

	seen := map[reflect.Type]bool{}
	var walk func(t reflect.Type, path string)
	walk = func(rt reflect.Type, path string) {
		if seen[rt] {
			return
		}
		seen[rt] = true
		for i := 0; i < rt.NumField(); i++ {
			f := rt.Field(i)
			ft := f.Type
			for ft.Kind() == reflect.Pointer || ft.Kind() == reflect.Slice || ft.Kind() == reflect.Map {
				ft = ft.Elem()
			}
			if ft.Kind() != reflect.Struct || ft.Name() == "" || ft.PkgPath() == "" {
				continue // a scalar, or a stdlib type like time.Time
			}
			where := path + "." + f.Name
			inline := isInline(f)
			// ChecksConfig is the module index: its fields are documented as
			// `checks.<module>` sections, not as a type of its own.
			if ft.Name() != "ChecksConfig" && !inline &&
				!strings.Contains(doc, "### `"+ft.Name()+"`") {
				t.Errorf("%s (%s) has no section in config-schema.md — its keys are undocumented",
					where, ft.Name())
			}
			// An inline struct contributes its keys to the parent's table, so
			// each of those keys must appear somewhere in the document.
			if inline {
				for j := 0; j < ft.NumField(); j++ {
					if key := yamlKey(ft.Field(j)); key != "" && !strings.Contains(doc, "`"+key+"`") {
						t.Errorf("%s: inline key %q from %s is missing from config-schema.md",
							where, key, ft.Name())
					}
				}
			}
			walk(ft, where)
		}
	}
	walk(reflect.TypeOf(engine.Config{}), "Config")

	if len(seen) < 30 {
		t.Fatalf("only %d types walked — the traversal is not reaching the module configs", len(seen))
	}
}
