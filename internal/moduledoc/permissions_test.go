package moduledoc

import (
	"strings"
	"testing"
)

// TestEveryDocumentedModuleHasPermissions is the coverage gate. A module that
// ships without a permissions entry is a module a security review cannot
// approve, and the answer "read the source" is what CF-181 exists to remove.
//
// Docs is the same map `checkfleet explain` and the SARIF rules read, so this
// compares the two halves of the single source against each other.
func TestEveryDocumentedModuleHasPermissions(t *testing.T) {
	if len(Docs) == 0 {
		t.Fatal("no modules documented")
	}
	for name := range Docs {
		if _, ok := Permissions[name]; !ok {
			t.Errorf("module %q has a description but no permissions entry", name)
		}
	}
	for name := range Permissions {
		if _, ok := Docs[name]; !ok {
			t.Errorf("permissions entry %q is not a documented module", name)
		}
	}
}

// TestEveryEntrySaysWhatIsNotNeeded — a permissions answer that lists only
// grants leaves a reviewer to assume the worst. The "not needed" half is the
// part that gets the ticket approved, so it is required, not optional.
func TestEveryEntrySaysWhatIsNotNeeded(t *testing.T) {
	for name, p := range Permissions {
		if strings.TrimSpace(p.Summary) == "" {
			t.Errorf("%s: empty summary", name)
		}
		if strings.TrimSpace(p.NotNeeded) == "" {
			t.Errorf("%s: no NotNeeded — say what the check cannot do, not only what it can", name)
		}
	}
}

// TestCredentialedModulesCarryStatements: where the system has a grant syntax,
// the operator should be able to copy it rather than translate prose.
func TestCredentialedModulesCarryStatements(t *testing.T) {
	// These have a real grant language and no excuse for prose alone.
	for _, name := range []string{"postgres", "mysql", "mongodb", "redis", "kafka", "elasticsearch", "clickhouse", "rabbitmq", "s3"} {
		p, ok := Permissions[name]
		if !ok {
			t.Fatalf("%s missing", name)
		}
		if p.Unauthenticated {
			t.Errorf("%s is marked unauthenticated but needs a credential", name)
		}
		if len(p.Statements) == 0 {
			t.Errorf("%s: no copy-pasteable statements", name)
		}
	}
}

// TestNoRealSecretsInStatements — these strings are printed by `checkfleet
// perms` and published on the docs site. A plausible-looking password in an
// example is the one that ends up in production.
func TestNoRealSecretsInStatements(t *testing.T) {
	for name, p := range Permissions {
		for _, s := range p.Statements {
			low := strings.ToLower(s)
			if !strings.Contains(low, "password") && !strings.Contains(low, "pwd") && !strings.Contains(low, "identified by") {
				continue
			}
			if !strings.Contains(s, "<from your secret store>") {
				t.Errorf("%s: a statement sets a credential without the placeholder: %q", name, s)
			}
		}
	}
}

// TestUnauthenticatedModulesClaimNoGrants keeps the two fields honest against
// each other: a module that needs no credential cannot also need a grant.
func TestUnauthenticatedModulesClaimNoGrants(t *testing.T) {
	for name, p := range Permissions {
		if p.Unauthenticated && len(p.Statements) > 0 {
			t.Errorf("%s: marked unauthenticated but carries grant statements", name)
		}
	}
}

func TestPermsLookup(t *testing.T) {
	if _, ok := Perms("postgres"); !ok {
		t.Error("postgres should be found")
	}
	if _, ok := Perms("nonexistent"); ok {
		t.Error("an unknown module must not be found")
	}
}
