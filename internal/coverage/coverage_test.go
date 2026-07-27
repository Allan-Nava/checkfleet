package coverage

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/inventory"
)

func TestHostsOf(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"https URL", "https://api.example.com/health", []string{"api.example.com"}},
		{"URL with port", "http://10.0.0.5:8080/", []string{"10.0.0.5"}},
		{"redis URL", "redis://cache1:6379", []string{"cache1"}},
		{"host:port", "db1.internal:5432", []string{"db1.internal"}},
		{"bare host", "github.com", []string{"github.com"}},
		{"IPv6", "[::1]:443", []string{"::1"}},
		{"libpq key=value DSN", "host=db1 user=app sslmode=require", []string{"db1"}},

		// The two shapes real config exposed, which the first implementation
		// got wrong: a replica-set URI names every member, and a Go MySQL DSN
		// hides the address inside net(...).
		{
			"mongodb replica set names every member",
			"mongodb://mongo-01:27017,mongo-02:27017,mongo-03:27017/?replicaSet=rs0",
			[]string{"mongo-01", "mongo-02", "mongo-03"},
		},
		{"Go MySQL DSN", "monitor:hunter2@tcp(db-01:3306)/", []string{"db-01"}},
		{"Go MySQL DSN, no creds", "tcp(db-02:3306)/app", []string{"db-02"}},

		{"empty", "", nil},
		{"an object key is not a host", "some/object", nil},
		{"a queue name with a space", "my queue", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hostsOf(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("hostsOf(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// Credentials must never survive into a Target: this list is printed to
// terminals, JSON files and CI logs.
func TestHostsOfDropsCredentials(t *testing.T) {
	cases := map[string]string{
		"postgres://app:sup3rsecret@db1:5432/prod?sslmode=require": "sup3rsecret",
		"mongodb://admin:hunter2@mongo1:27017/admin":               "hunter2",
		"monitor:toor@tcp(mysql1:3306)/app":                        "toor",
		"https://user:pass@api.example.com/":                       "pass",
	}
	for dsn, secret := range cases {
		hosts := hostsOf(dsn)
		if len(hosts) == 0 {
			t.Errorf("hostsOf(%q) found no host", dsn)
		}
		for _, h := range hosts {
			if strings.Contains(h, secret) {
				t.Errorf("hostsOf(%q) leaked %q in %q", dsn, secret, h)
			}
		}
	}
	// safeName, the display-label path, must be just as careful.
	if n := safeName("postgres://app:sup3rsecret@db1:5432/prod"); strings.Contains(n, "sup3rsecret") {
		t.Errorf("safeName leaked a credential: %q", n)
	}
}

// A whole-config end-to-end check that no credential reaches any Target field.
func TestTargetsNeverCarryCredentials(t *testing.T) {
	cfg := &engine.Config{}
	cfg.Checks.Postgres = &engine.PostgresConfig{Targets: []engine.PostgresTarget{
		{Name: "pg", DSN: "postgres://app:sup3rsecret@db1:5432/prod"},
	}}
	cfg.Checks.MySQL = &engine.MySQLConfig{Targets: []engine.MySQLTarget{
		{Name: "my", DSN: "monitor:toor@tcp(db-01:3306)/"},
	}}
	cfg.Checks.MongoDB = &engine.MongoDBConfig{Targets: []engine.MongoDBTarget{
		{Name: "mongo", URI: "mongodb://admin:hunter2@mongo1:27017/admin"},
	}}

	for _, x := range Targets(cfg) {
		blob := x.Name + " " + strings.Join(x.Hosts, " ")
		for _, secret := range []string{"sup3rsecret", "toor", "hunter2"} {
			if strings.Contains(blob, secret) {
				t.Errorf("module %q leaked %q: %+v", x.Module, secret, x)
			}
		}
	}
}

func TestTargetsFlattensModules(t *testing.T) {
	cfg := &engine.Config{}
	cfg.Checks.Certs = &engine.CertsConfig{Targets: []string{"github.com", "api.example.com"}}
	cfg.Checks.HTTP = &engine.HTTPConfig{Targets: []engine.HTTPTarget{
		{URL: "https://example.com/health"},
	}}
	cfg.Checks.Kafka = &engine.KafkaConfig{Brokers: []string{"kafka1:9092"}}
	cfg.Checks.Keycloak = &engine.KeycloakConfig{BaseURL: "https://sso.example.com"}

	got := Targets(cfg)
	if len(got) != 5 {
		t.Fatalf("got %d targets, want 5: %+v", len(got), got)
	}

	byModule := map[string][]Target{}
	for _, x := range got {
		byModule[x.Module] = append(byModule[x.Module], x)
	}
	if n := len(byModule["certs"]); n != 2 {
		t.Errorf("certs: %d targets, want 2", n)
	}
	// A []string module: the string is both name and host.
	if c := byModule["certs"][0]; c.Name != "github.com" || !hasHost(c, "github.com") {
		t.Errorf("certs target = %+v", c)
	}
	// A struct module with no Name: the URL is the label, the host is extracted.
	if h := byModule["http"][0]; h.Name != "https://example.com/health" || !hasHost(h, "example.com") {
		t.Errorf("http target = %+v", h)
	}
	// Brokers, not Targets.
	if k := byModule["kafka"][0]; !hasHost(k, "kafka1") {
		t.Errorf("kafka target = %+v", k)
	}
	// A bare string field, not a slice.
	if k := byModule["keycloak"][0]; !hasHost(k, "sso.example.com") {
		t.Errorf("keycloak target = %+v", k)
	}
}

func hasHost(t Target, host string) bool {
	for _, h := range t.Hosts {
		if h == host {
			return true
		}
	}
	return false
}

func TestTargetsSkipsUnconfiguredModules(t *testing.T) {
	cfg := &engine.Config{}
	cfg.Checks.Certs = &engine.CertsConfig{Targets: []string{"github.com"}}
	for _, x := range Targets(cfg) {
		if x.Module != "certs" {
			t.Errorf("unconfigured module %q produced a target", x.Module)
		}
	}
	if got := Targets(&engine.Config{}); len(got) != 0 {
		t.Errorf("an empty config yielded %d targets", len(got))
	}
	if got := Targets(nil); got != nil {
		t.Error("a nil config should yield no targets")
	}
}

// The guard for the generic enumeration: configure EVERY module and assert each
// one contributes at least one target. Without this, adding a module whose
// shape doesn't match the convention makes coverage silently under-report —
// the one failure mode that matters for a tool answering "is it all monitored?".
func TestEveryModuleYieldsTargets(t *testing.T) {
	cfg := &engine.Config{}
	checks := reflect.ValueOf(&cfg.Checks).Elem()
	ct := checks.Type()

	var modules []string
	for i := 0; i < checks.NumField(); i++ {
		f := checks.Field(i)
		if f.Kind() != reflect.Pointer {
			t.Fatalf("field %s is not a pointer; the enumeration assumes it is", ct.Field(i).Name)
		}
		// Allocate the module config and fill its target field with one entry.
		f.Set(reflect.New(f.Type().Elem()))
		if !fillOneTarget(f.Elem()) {
			t.Errorf("module %q: no field named %v that coverage can enumerate — "+
				"add it to targetFields or give the module a Targets field",
				yamlName(ct.Field(i)), targetFields)
			continue
		}
		modules = append(modules, yamlName(ct.Field(i)))
	}

	got := Targets(cfg)
	seen := map[string]int{}
	for _, x := range got {
		seen[x.Module]++
		if x.Name == "" {
			t.Errorf("module %q produced a target with no name: %+v", x.Module, x)
		}
		if len(x.Hosts) == 0 {
			t.Errorf("module %q produced a target with no host, so it can never "+
				"match an inventory: %+v", x.Module, x)
		}
	}
	for _, m := range modules {
		if seen[m] == 0 {
			t.Errorf("module %q is configured but produced no targets", m)
		}
	}
	if len(modules) < 29 {
		t.Errorf("only %d modules walked, expected at least 29", len(modules))
	}
}

// fillOneTarget puts a single plausible entry in the module's target field.
func fillOneTarget(cfg reflect.Value) bool {
	for _, name := range targetFields {
		f := cfg.FieldByName(name)
		if !f.IsValid() || !f.CanSet() {
			continue
		}
		switch f.Kind() {
		case reflect.String:
			f.SetString("https://host.example.com")
			return true
		case reflect.Slice:
			elem := reflect.New(f.Type().Elem()).Elem()
			switch elem.Kind() {
			case reflect.String:
				elem.SetString("host.example.com:443")
			case reflect.Struct:
				// Set whichever address field this target struct has, so the
				// entry looks like a real one.
				set := false
				for _, an := range addressFields {
					if af := elem.FieldByName(an); af.IsValid() && af.Kind() == reflect.String {
						af.SetString("host.example.com:443")
						set = true
						break
					}
				}
				if nf := elem.FieldByName("Name"); nf.IsValid() && nf.Kind() == reflect.String {
					nf.SetString("host.example.com")
					set = true
				}
				if !set {
					return false
				}
			default:
				return false
			}
			f.Set(reflect.Append(f, elem))
			return true
		}
	}
	return false
}

func TestDiffInventory(t *testing.T) {
	targets := []Target{
		{Module: "http", Name: "https://web1/", Hosts: []string{"web1"}},
		{Module: "certs", Name: "10.0.0.5", Hosts: []string{"10.0.0.5"}},
		{Module: "certs", Name: "github.com", Hosts: []string{"github.com"}},
		{Module: "consul", Name: "some-kv-key"}, // no host: ignored
	}
	hosts := []inventory.Host{
		{Name: "web1", Address: "web1", Group: "web"},
		{Name: "web2", Address: "10.0.0.5", Group: "web"}, // matched by ansible_host
		{Name: "db1", Address: "db1", Group: "db"},        // not monitored
	}

	d := DiffInventory(targets, hosts)
	if len(d.Covered) != 2 || d.Covered[0] != "web1" || d.Covered[1] != "web2" {
		t.Errorf("covered = %v, want [web1 web2] (web2 matched via ansible_host)", d.Covered)
	}
	if len(d.Uncovered) != 1 || d.Uncovered[0] != "db1" {
		t.Errorf("uncovered = %v, want [db1]", d.Uncovered)
	}
	// github.com is a real target but not an inventory host: an external
	// dependency, reported separately rather than as a problem.
	if len(d.Extra) != 1 || d.Extra[0] != "github.com" {
		t.Errorf("extra = %v, want [github.com]", d.Extra)
	}
}

// One target naming several hosts covers all of them — the replica-set case.
func TestDiffInventoryMultiHostTargetCoversEveryMember(t *testing.T) {
	targets := []Target{{
		Module: "mongodb",
		Name:   "primary-rs",
		Hosts:  []string{"mongo-01", "mongo-02", "mongo-03"},
	}}
	hosts := []inventory.Host{
		{Name: "mongo-01", Address: "mongo-01"},
		{Name: "mongo-02", Address: "mongo-02"},
		{Name: "mongo-03", Address: "mongo-03"},
	}
	d := DiffInventory(targets, hosts)
	if len(d.Uncovered) != 0 {
		t.Errorf("a replica-set URI must cover every member, uncovered = %v", d.Uncovered)
	}
	if len(d.Covered) != 3 {
		t.Errorf("covered = %v, want all three members", d.Covered)
	}
}

func TestDiffInventoryIsCaseInsensitive(t *testing.T) {
	targets := []Target{{Module: "certs", Name: "WEB1.example.com", Hosts: []string{"WEB1.example.com"}}}
	hosts := []inventory.Host{{Name: "web1.example.com", Address: "web1.example.com"}}
	if d := DiffInventory(targets, hosts); len(d.Covered) != 1 {
		t.Errorf("hostnames are case-insensitive, got %+v", d)
	}
}

func TestDiffInventoryEmptyCases(t *testing.T) {
	hosts := []inventory.Host{{Name: "web1", Address: "web1"}}
	d := DiffInventory(nil, hosts)
	if len(d.Uncovered) != 1 {
		t.Errorf("with no targets every host is uncovered, got %+v", d)
	}
	d = DiffInventory([]Target{{Module: "http", Name: "x", Hosts: []string{"web1"}}}, nil)
	if len(d.Extra) != 1 || len(d.Covered) != 0 {
		t.Errorf("with no inventory every target host is extra, got %+v", d)
	}
}
