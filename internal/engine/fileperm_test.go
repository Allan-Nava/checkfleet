package engine

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func secretAt(t *testing.T, mode os.FileMode) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(p, []byte("s3cret\n"), mode); err != nil {
		t.Fatal(err)
	}
	// WriteFile is subject to umask, so set the mode explicitly.
	if err := os.Chmod(p, mode); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCheckSecretFileAcceptsTightModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix mode bits are meaningless here")
	}
	for _, mode := range []os.FileMode{0o600, 0o400, 0o640} {
		if err := CheckSecretFile(secretAt(t, mode)); err != nil {
			t.Errorf("mode %04o should be accepted: %v", mode, err)
		}
	}
}

// TestGroupReadableIsAllowed — running under a dedicated group with the secret
// at 0640 is a normal deployment. Refusing it would push people back to putting
// the password in a unit file, which is not an improvement.
func TestGroupReadableIsAllowed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix mode bits are meaningless here")
	}
	if err := CheckSecretFile(secretAt(t, 0o640)); err != nil {
		t.Errorf("0640 should be allowed: %v", err)
	}
}

func TestCheckSecretFileRefusesWorldReadable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix mode bits are meaningless here")
	}
	for _, mode := range []os.FileMode{0o644, 0o666, 0o777, 0o604} {
		err := CheckSecretFile(secretAt(t, mode))
		if err == nil {
			t.Errorf("mode %04o must be refused", mode)
			continue
		}
		// The fix belongs in the message: "your permissions are wrong" without
		// the command wastes someone's afternoon.
		if !strings.Contains(err.Error(), "chmod 0600") {
			t.Errorf("mode %04o: the error should name the chmod: %v", mode, err)
		}
	}
}

func TestCheckSecretFileReportsAMissingFile(t *testing.T) {
	if err := CheckSecretFile(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("a missing file must be an error")
	}
}

// TestFileInterpolationRefusesAWorldReadableSecret is the behaviour that
// matters: reading it in silence is how a world-readable credential stays wrong
// for a year.
func TestFileInterpolationRefusesAWorldReadableSecret(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix mode bits are meaningless here")
	}
	bad := secretAt(t, 0o644)
	_, err := LoadBytes([]byte("checks:\n  redis:\n    password_env: X\n    targets: [\"${file:" + bad + "}\"]\n"))
	if err == nil {
		t.Fatal("a world-readable secret file must be refused")
	}
	if !strings.Contains(err.Error(), "readable by other users") {
		t.Errorf("the error should say why: %v", err)
	}
}

func TestFileInterpolationAcceptsATightSecret(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix mode bits are meaningless here")
	}
	good := secretAt(t, 0o600)
	cfg, err := LoadBytes([]byte("checks:\n  redis:\n    targets: [\"${file:" + good + "}\"]\n"))
	if err != nil {
		t.Fatalf("a 0600 secret file should be read: %v", err)
	}
	if len(cfg.Checks.Redis.Targets) != 1 || cfg.Checks.Redis.Targets[0] != "s3cret" {
		t.Errorf("interpolation did not happen: %+v", cfg.Checks.Redis.Targets)
	}
}

func TestFileIsWorldReadableIsAdvisory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix mode bits are meaningless here")
	}
	world, mode, ok := FileIsWorldReadable(secretAt(t, 0o644))
	if !ok || !world || mode != 0o644 {
		t.Errorf("got world=%v mode=%04o ok=%v", world, mode, ok)
	}
	tight, _, ok := FileIsWorldReadable(secretAt(t, 0o600))
	if !ok || tight {
		t.Error("0600 must not be reported as world-readable")
	}
	// A missing file is not a permission problem; doctor should stay quiet.
	if _, _, ok := FileIsWorldReadable("/nope/nope"); ok {
		t.Error("a missing file should report ok=false, not a warning")
	}
}
