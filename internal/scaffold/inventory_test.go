package scaffold

import (
	"strings"
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/inventory"
)

func webHosts() []inventory.Host {
	return []inventory.Host{
		{Name: "web2", Address: "web2", Group: "web"},
		{Name: "web1", Address: "10.0.0.5", Group: "web"}, // via ansible_host
	}
}

func TestFromInventoryUsesEveryHost(t *testing.T) {
	got, err := FromInventory(webHosts(), []string{"certs", "http"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"10.0.0.5:443", "web2:443", "https://10.0.0.5/", "https://web2/"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "2 host(s) from group(s): web") {
		t.Errorf("the header should say what it scaffolded from:\n%s", got)
	}
}

// Regenerating from the same inventory must produce the same bytes, so a diff
// shows real inventory changes and not map iteration order.
func TestFromInventoryIsDeterministic(t *testing.T) {
	first, err := FromInventory(webHosts(), []string{"certs", "http", "tcp"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		again, err := FromInventory(webHosts(), []string{"certs", "http", "tcp"})
		if err != nil {
			t.Fatal(err)
		}
		if again != first {
			t.Fatal("output is not stable across runs")
		}
	}
	// Hosts are sorted by name, so web1 precedes web2 whatever the input order.
	// Checked in the tcp block, the one that prints both names (elsewhere web1
	// appears as its ansible_host address, 10.0.0.5).
	if strings.Index(first, "name: web1") > strings.Index(first, "name: web2") {
		t.Errorf("hosts should be sorted by name:\n%s", first)
	}
}

// A module needing more than a hostname must be refused with an explanation,
// not scaffolded with an invented DSN that fails on the first run.
func TestFromInventoryRefusesUnderivableModule(t *testing.T) {
	_, err := FromInventory(webHosts(), []string{"postgres"})
	if err == nil {
		t.Fatal("postgres cannot be derived from an inventory; it must be refused")
	}
	if !strings.Contains(err.Error(), "cannot be derived") {
		t.Errorf("the error should explain why: %v", err)
	}
	// And it should point at what does work.
	if !strings.Contains(err.Error(), "certs") {
		t.Errorf("the error should list the inventory-capable modules: %v", err)
	}
}

func TestFromInventoryErrors(t *testing.T) {
	if _, err := FromInventory(nil, []string{"certs"}); err == nil {
		t.Error("no hosts should be an error")
	}
	if _, err := FromInventory(webHosts(), []string{"nosuchmodule"}); err == nil {
		t.Error("an unknown module should be an error")
	}
}

func TestFromInventoryDefaultModules(t *testing.T) {
	got, err := FromInventory(webHosts(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "certs:") || !strings.Contains(got, "http:") {
		t.Errorf("the default set should be certs+http:\n%s", got)
	}
}

// Nothing secret may appear in a generated config.
func TestFromInventoryEmitsNoSecrets(t *testing.T) {
	got, err := FromInventory(webHosts(), InventoryModules())
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"password:", "passwd", "secret:", "token:"} {
		if strings.Contains(strings.ToLower(got), forbidden) {
			t.Errorf("generated config contains %q:\n%s", forbidden, got)
		}
	}
}

// Every module FromInventory claims to support must actually render something
// for each host — a silently empty block would produce a config that checks
// nothing.
func TestEveryInventoryModuleRenders(t *testing.T) {
	hosts := webHosts()
	for _, m := range InventoryModules() {
		got, err := FromInventory(hosts, []string{m})
		if err != nil {
			t.Errorf("module %q: %v", m, err)
			continue
		}
		if !strings.Contains(got, "  "+m+":") {
			t.Errorf("module %q produced no block:\n%s", m, got)
		}
		for _, h := range hosts {
			if !strings.Contains(got, h.Address) && !strings.Contains(got, h.Name) {
				t.Errorf("module %q dropped host %+v:\n%s", m, h, got)
			}
		}
	}
}
