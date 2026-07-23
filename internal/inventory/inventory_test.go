package inventory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseIniInventory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "hosts")
	content := `
[web]
web-01  ansible_host=10.0.0.1  ansible_user=root
web-02  ansible_host=10.0.0.2   # commento dopo l'host

[db]
db-01 ansible_host=10.0.1.1
bare-host

[prod:children]
web
db

[prod:vars]
env=production
`
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	hosts, err := LoadPath(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 4 {
		t.Fatalf("attesi 4 host, avuti %d: %+v", len(hosts), hosts)
	}
	byName := map[string]Host{}
	for _, h := range hosts {
		byName[h.Name] = h
	}
	if byName["web-01"].Address != "10.0.0.1" {
		t.Errorf("web-01 address: %s", byName["web-01"].Address)
	}
	if byName["bare-host"].Address != "bare-host" {
		t.Errorf("host senza ansible_host deve usare il nome: %s", byName["bare-host"].Address)
	}
	if _, ok := byName["web"]; ok {
		t.Error("le righe di [prod:children] non sono host")
	}
	if _, ok := byName["env=production"]; ok {
		t.Error("le righe di [prod:vars] non sono host")
	}
}

func TestLoadDirectory(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "a"), []byte("[g1]\nh1 ansible_host=1.1.1.1\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "b"), []byte("[g2]\nh2\nh1 ansible_host=9.9.9.9\n"), 0o644)
	hosts, err := LoadPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 2 {
		t.Fatalf("attesi 2 host dedupati, avuti %d: %+v", len(hosts), hosts)
	}
}
