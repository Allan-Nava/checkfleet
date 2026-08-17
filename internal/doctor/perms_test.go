package doctor

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

func fileAt(t *testing.T, name string, mode os.FileMode) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte("x\n"), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, mode); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFilePermsWarnsOnAWorldReadableConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix mode bits are meaningless here")
	}
	got := FilePerms(fileAt(t, "checkfleet.yml", 0o644), "")
	if len(got) != 1 || got[0].Status != engine.WARN {
		t.Fatalf("want one WARN, got %+v", got)
	}
	// The fix in the message, not just the diagnosis.
	if !strings.Contains(got[0].Message, "chmod 0600") {
		t.Errorf("the warning should name the chmod: %s", got[0].Message)
	}
}

// TestFilePermsIsAdvisory — doctor reports, it never refuses. A permission that
// is wrong today should not take the monitoring down with it.
func TestFilePermsIsAdvisory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix mode bits are meaningless here")
	}
	for _, f := range FilePerms(fileAt(t, "checkfleet.yml", 0o666), "") {
		if f.Status == engine.BAD || f.Status == engine.ERROR {
			t.Errorf("a permission problem must be WARN, not %s", f.Status)
		}
	}
}

func TestFilePermsAcceptsATightConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix mode bits are meaningless here")
	}
	got := FilePerms(fileAt(t, "checkfleet.yml", 0o600), "")
	if len(got) != 1 || got[0].Status != engine.OK {
		t.Fatalf("want one OK, got %+v", got)
	}
}

// TestFilePermsCoversTheSecretFilesToo: the config may be tight while the file
// it reads the password from is not, and that is the more dangerous half.
func TestFilePermsCoversTheSecretFilesToo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix mode bits are meaningless here")
	}
	cfg := fileAt(t, "checkfleet.yml", 0o600)
	secret := fileAt(t, "pg.pass", 0o644)
	raw := "checks:\n  postgres:\n    targets: [{dsn: \"${file:" + secret + "}\"}]\n"

	got := FilePerms(cfg, raw)
	var warned bool
	for _, f := range got {
		if f.Target == secret && f.Status == engine.WARN {
			warned = true
		}
	}
	if !warned {
		t.Errorf("the referenced secret file was not checked: %+v", got)
	}
}

func TestFilePermsReportsEachPathOnce(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix mode bits are meaningless here")
	}
	secret := fileAt(t, "s.pass", 0o644)
	raw := "a: ${file:" + secret + "}\nb: ${file:" + secret + "}\n"
	got := FilePerms(fileAt(t, "c.yml", 0o600), raw)
	seen := 0
	for _, f := range got {
		if f.Target == secret {
			seen++
		}
	}
	if seen != 1 {
		t.Errorf("the same file was reported %d times", seen)
	}
}

// TestFilePermsStaysQuietAboutMissingFiles — a path that does not exist is a
// config error for someone else to report, not a permission warning.
func TestFilePermsStaysQuietAboutMissingFiles(t *testing.T) {
	got := FilePerms("/nope/checkfleet.yml", "x: ${file:/nope/secret}\n")
	if len(got) != 0 {
		t.Errorf("want no findings for missing files, got %+v", got)
	}
}
