package doctor

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

const checkPerms = "doctor/perms"

// fileRefPattern finds the ${file:/path} tokens a raw config uses as secret
// sources, so their permissions can be reported alongside the config's own.
var fileRefPattern = regexp.MustCompile(`\$\{file:([^}]+)\}`)

// FilePerms warns about a config — or a file it reads a secret from — that other
// accounts on the host can read (CF-185).
//
// A checkfleet.yml matters even when it holds only `*_env` keys: it maps out the
// fleet, names every host and port, and says which credential lives in which
// variable. That is a reconnaissance document.
//
// Advisory on purpose. `doctor` reports; it does not refuse to run, because a
// permission that is wrong today should not take the monitoring down with it —
// the run is how you find out something else broke. The one place checkfleet
// does refuse is reading a world-readable file as a secret, which is a
// different bargain: there, continuing means using the credential anyway.
func FilePerms(configPath, rawConfig string) []engine.Finding {
	var out []engine.Finding
	seen := map[string]bool{}

	add := func(path, what string) {
		if path == "" || seen[path] {
			return
		}
		seen[path] = true
		world, mode, ok := engine.FileIsWorldReadable(path)
		if !ok {
			return // missing or not a Unix filesystem: not a permission problem
		}
		if !world {
			out = append(out, engine.Finding{
				Check: checkPerms, Target: path, Status: engine.OK,
				Message: fmt.Sprintf("mode %04o", mode),
			})
			return
		}
		out = append(out, engine.Finding{
			Check: checkPerms, Target: path, Status: engine.WARN,
			Message: fmt.Sprintf("%s is world-readable (mode %04o); run: chmod 0600 %s", what, mode, path),
		})
	}

	add(configPath, "the config")
	for _, m := range fileRefPattern.FindAllStringSubmatch(rawConfig, -1) {
		add(strings.TrimSpace(m[1]), "the secret file")
	}
	return out
}
