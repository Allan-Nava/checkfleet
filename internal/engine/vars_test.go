package engine

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeVarFile writes body to an absolute path (include_test.go's writeFile
// takes a dir + name instead).
func writeVarFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func timeoutAfterSeconds(n int) <-chan time.Time {
	return time.After(time.Duration(n) * time.Second)
}

func refByName(refs []VarRef, name string) (VarRef, bool) {
	for _, r := range refs {
		if r.Name == name {
			return r, true
		}
	}
	return VarRef{}, false
}

func TestScanVarsKinds(t *testing.T) {
	t.Setenv("CF_VARS_SET", "x")

	dir := t.TempDir()
	secret := filepath.Join(dir, "secret.txt")
	writeVarFile(t, secret, "s3cret\n")

	path := filepath.Join(dir, "checkfleet.yml")
	writeVarFile(t, path, `timeout_seconds: 5
a: "${CF_VARS_SET}"
b: "${CF_VARS_UNSET}"
c: "${CF_VARS_DEFAULTED:-fallback}"
d: "${file:`+secret+`}"
e: "${file:`+filepath.Join(dir, "nope.txt")+`}"
`)

	refs, err := ScanVars(path)
	if err != nil {
		t.Fatal(err)
	}

	if r, ok := refByName(refs, "CF_VARS_SET"); !ok || !r.Resolved || r.Kind != VarEnv {
		t.Errorf("CF_VARS_SET = %+v", r)
	}
	if r, ok := refByName(refs, "CF_VARS_UNSET"); !ok || !r.Missing() {
		t.Errorf("CF_VARS_UNSET should be missing: %+v", r)
	}
	if r, ok := refByName(refs, "CF_VARS_DEFAULTED"); !ok || !r.HasDefault || r.Missing() {
		t.Errorf("a defaulted var is not missing: %+v", r)
	}
	if r, ok := refByName(refs, secret); !ok || r.Kind != VarFile || !r.Resolved {
		t.Errorf("readable secret file = %+v", r)
	}
	if r, ok := refByName(refs, filepath.Join(dir, "nope.txt")); !ok || !r.Missing() {
		t.Errorf("unreadable secret file should be missing: %+v", r)
	}
}

// $${ is the literal escape, not a reference — expandVars treats it that way,
// so the scanner must too or it would invent a missing variable.
func TestScanVarsIgnoresEscapedDollar(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "checkfleet.yml")
	writeVarFile(t, path, "a: \"$${NOT_A_VAR}\"\nb: \"${CF_REAL_VAR}\"\n")

	refs, err := ScanVars(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := refByName(refs, "NOT_A_VAR"); ok {
		t.Error("$${...} is a literal and must not be reported as a variable")
	}
	if _, ok := refByName(refs, "CF_REAL_VAR"); !ok {
		t.Error("the real reference was missed")
	}
}

// The same variable used by three targets is one thing to fix.
func TestScanVarsDeduplicatesPerFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "checkfleet.yml")
	writeVarFile(t, path, "a: \"${CF_SAME}\"\nb: \"${CF_SAME}\"\nc: \"${CF_SAME}\"\n")

	refs, err := ScanVars(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 {
		t.Errorf("got %d refs, want 1 deduplicated: %+v", len(refs), refs)
	}
}

// Variables hiding in an included file still get reported, with the file that
// contains them.
func TestScanVarsFollowsIncludes(t *testing.T) {
	dir := t.TempDir()
	inc := filepath.Join(dir, "extra.yml")
	writeVarFile(t, inc, "checks:\n  redis:\n    password_env: \"${CF_IN_INCLUDE}\"\n")

	path := filepath.Join(dir, "checkfleet.yml")
	writeVarFile(t, path, "include: extra.yml\na: \"${CF_IN_BASE}\"\n")

	refs, err := ScanVars(path)
	if err != nil {
		t.Fatal(err)
	}
	base, ok := refByName(refs, "CF_IN_BASE")
	if !ok || base.File != path {
		t.Errorf("base ref = %+v, want file %s", base, path)
	}
	sub, ok := refByName(refs, "CF_IN_INCLUDE")
	if !ok {
		t.Fatal("a variable in an included file was missed")
	}
	if sub.File != inc {
		t.Errorf("included ref reported in %q, want %q", sub.File, inc)
	}
}

// ScanVars must work on a config that cannot be loaded — that is when a
// diagnostic is needed. Here the include target does not exist.
func TestScanVarsSurvivesBrokenConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "checkfleet.yml")
	writeVarFile(t, path, "include: does-not-exist.yml\na: \"${CF_STILL_FOUND}\"\n")

	refs, err := ScanVars(path)
	if err != nil {
		t.Fatalf("a broken include must not stop the scan: %v", err)
	}
	if _, ok := refByName(refs, "CF_STILL_FOUND"); !ok {
		t.Error("the reference in the unloadable config was not reported")
	}
}

func TestScanVarsMissingFile(t *testing.T) {
	if _, err := ScanVars(filepath.Join(t.TempDir(), "nope.yml")); err == nil {
		t.Error("a missing config file should be an error")
	}
}

func TestScanVarsIncludeCycle(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.yml")
	b := filepath.Join(dir, "b.yml")
	writeVarFile(t, a, "include: b.yml\n")
	writeVarFile(t, b, "include: a.yml\n")

	// The cycle must not hang or recurse forever; the loader reports it, and the
	// scan simply stops descending.
	done := make(chan struct{})
	go func() {
		_, _ = ScanVars(a)
		close(done)
	}()
	select {
	case <-done:
	case <-timeoutAfterSeconds(5):
		t.Fatal("ScanVars did not terminate on an include cycle")
	}
}
