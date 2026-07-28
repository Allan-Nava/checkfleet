package main

import (
	"fmt"
	"io"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// warnUnknownKeys prints a notice for every config key YAML silently ignores —
// in the base config, in the stack overlays, and in anything they include.
//
// `validate` and `doctor` already report these, but only to whoever runs them. A
// scheduled `check` would keep reporting a *healthy* fleet while a misspelled
// module never ran, which is the worst outcome this tool can produce: not a
// wrong answer, a confident one. So the run says it too.
//
// Two deliberate limits (see docs/compatibility.md): unknown keys do **not**
// abort the run — a config written for a newer checkfleet must keep working on
// an older one — and they do **not** change the exit code, because a typo is not
// a systemic failure. The notice goes to stderr so it can never end up inside a
// rendered document or a webhook payload.
func warnUnknownKeys(w io.Writer, path, stack string) {
	paths := []string{path}
	for _, s := range splitStacks(stack) {
		paths = append(paths, engine.StackPath(path, s))
	}

	seen := map[string]bool{}
	for _, p := range paths {
		for _, problem := range engine.UnknownKeys(p) {
			// The base config and a stack overlay can include the same drop-in
			// file; saying it twice would just look like a bug.
			line := problem.String()
			if seen[line] {
				continue
			}
			seen[line] = true
			fmt.Fprintf(w, "checkfleet: warning: %s\n", line)
		}
	}
}
