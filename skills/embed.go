// Package skills embeds the agent skill shipped with the binary (CF-151), so
// whoever has checkfleet also has the skill at the matching version without
// cloning the repo.
//
// The embedding file lives here, next to the skill source, on purpose: go:embed
// cannot reach above its own package directory, and keeping the source at
// skills/checkfleet/ (where CF-149 put it) is worth more than keeping this file
// under internal/ — the alternative was a second copy of SKILL.md that would
// drift from the first.
package skills

import "embed"

// FS holds skills/checkfleet: SKILL.md plus the generated references/.
//
//go:embed checkfleet
var FS embed.FS

// Root is the directory inside FS, and the name the skill installs under.
const Root = "checkfleet"
