package engine

import (
	"fmt"
	"io/fs"
	"os"
	"runtime"
)

// SecretFileMode is the widest permission a file used as a secret source may
// have. Anything readable by "others" is refused: a password in a world-
// readable file is available to every account on the host, which is the whole
// threat the file was supposed to avoid.
//
// Group-readable is allowed on purpose. Running checkfleet under a dedicated
// group with the secret at 0640 is a normal, defensible deployment, and
// refusing it would push people back to environment variables in a unit file —
// which is not an improvement.
const SecretFileMode fs.FileMode = 0o037 // bits that must NOT be set (o+rwx, g+wx)

// CheckSecretFile reports whether path is safe to read a credential from
// (CF-185). It returns an error naming the exact chmod to run, because "your
// permissions are wrong" without the fix is a message that wastes someone's
// afternoon.
//
// On Windows the Unix mode bits carry no meaning, so the check is skipped
// rather than guessed at.
func CheckSecretFile(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	mode := fi.Mode().Perm()
	if mode&SecretFileMode == 0 {
		return nil
	}
	return fmt.Errorf("%s is readable by other users (mode %04o); run: chmod 0600 %s",
		path, mode, path)
}

// FileIsWorldReadable reports the same condition without an error, for the
// advisory path in `doctor` — which warns rather than blocks.
func FileIsWorldReadable(path string) (bool, fs.FileMode, bool) {
	if runtime.GOOS == "windows" {
		return false, 0, false
	}
	fi, err := os.Stat(path)
	if err != nil {
		return false, 0, false
	}
	mode := fi.Mode().Perm()
	return mode&0o004 != 0, mode, true
}
