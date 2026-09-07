package coverage

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

func TestDiscoveredExpandsInventoryHosts(t *testing.T) {
	inv := filepath.Join(t.TempDir(), "hosts.ini")
	if err := os.WriteFile(inv, []byte("web1 ansible_host=10.0.0.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := &engine.Config{Checks: engine.ChecksConfig{
		Certs: &engine.CertsConfig{Discovery: engine.Discovery{AnsibleInventory: inv}},
	}}

	got, errs := Discovered(context.Background(), cfg)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(got) != 1 || got[0].Module != "certs" || got[0].Name != "web1" {
		t.Fatalf("want the inventory host attributed to certs, got %+v", got)
	}
	if got[0].Source != "inventory" {
		t.Errorf("the source must survive into the target, got %q", got[0].Source)
	}
}

func TestDiscoveredReportsPerModuleErrors(t *testing.T) {
	cfg := &engine.Config{Checks: engine.ChecksConfig{
		Certs: &engine.CertsConfig{Discovery: engine.Discovery{
			AnsibleInventory: filepath.Join(t.TempDir(), "nope.ini"),
		}},
	}}
	_, errs := Discovered(context.Background(), cfg)
	if errs["certs"] == nil {
		t.Fatalf("want the failure attributed to certs, got %v", errs)
	}
}

// The guard against a silent gap: Discovered finds modules by reflection, so a
// module whose discovery field is named or typed differently would be skipped
// without a word — and `targets` would under-report its fleet. This pins the
// convention instead of the current count, so adding a module to the set is
// enough and renaming the field is caught.
func TestEveryModuleWithDiscoveryIsReached(t *testing.T) {
	checks := reflect.ValueOf(engine.ChecksConfig{})
	ct := checks.Type()
	want := reflect.TypeOf(engine.Discovery{})

	var found []string
	for i := 0; i < ct.NumField(); i++ {
		mt := ct.Field(i).Type.Elem() // every module config is a pointer
		for j := 0; j < mt.NumField(); j++ {
			f := mt.Field(j)
			if f.Type != want {
				continue
			}
			if f.Name != "Discovery" {
				t.Errorf("%s embeds engine.Discovery as %q; Discovered() looks it up by name and would skip it",
					mt.Name(), f.Name)
			}
			if tag := f.Tag.Get("yaml"); tag != ",inline" {
				t.Errorf("%s.Discovery must be yaml:\",inline\" so ansible_inventory keeps its key, got %q",
					mt.Name(), tag)
			}
			found = append(found, mt.Name())
		}
	}
	if len(found) == 0 {
		t.Fatal("no module carries engine.Discovery — the reflection convention is broken")
	}
}
