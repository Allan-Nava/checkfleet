package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// VarKind distinguishes the two interpolation sources a config can use.
type VarKind string

const (
	VarEnv  VarKind = "env"  // ${NAME} or ${NAME:-default}
	VarFile VarKind = "file" // ${file:/path/to/secret}
)

// VarRef is one ${...} reference found in a config file.
//
// This exists because an unset ${NAME} expands to the **empty string, silently**
// (see expandVars). The config still parses, the check still runs, and it fails
// against an empty password or an empty URL — with an error that points at the
// service instead of at the missing variable. Scanning the raw text is the only
// way to name the variable that is actually missing.
type VarRef struct {
	Name       string  `json:"name"`    // NAME, or the path for a file ref
	Kind       VarKind `json:"kind"`    //
	File       string  `json:"file"`    // config file it appears in
	HasDefault bool    `json:"default"` // written as ${NAME:-fallback}
	Resolved   bool    `json:"resolved"`
}

// Missing reports whether this reference will silently produce an empty value:
// an env var that is unset (or empty) with no default, or a secret file that
// cannot be read.
func (r VarRef) Missing() bool { return !r.Resolved && !r.HasDefault }

// ScanVars walks a config file and its `include:` chain and returns every
// ${...} reference, resolved or not, in file order.
//
// It reads the raw bytes rather than a parsed Config on purpose: by the time a
// config is parsed the references are gone, replaced by their values (or by
// nothing). It also means ScanVars still works on a config that fails to load,
// which is exactly when someone runs a diagnostic.
func ScanVars(path string) ([]VarRef, error) {
	return scanVars(path, map[string]bool{})
}

func scanVars(path string, visiting map[string]bool) ([]VarRef, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	if visiting[abs] {
		return nil, fmt.Errorf("config: include cycle at %s", path)
	}
	visiting[abs] = true
	defer delete(visiting, abs)

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	refs := refsIn(string(raw), path)

	// The include list is read from the *expanded* text, since an include path
	// may itself be interpolated. A failure here is not fatal to the scan: the
	// references found so far are still worth reporting, and they are usually
	// the reason the expansion failed in the first place.
	expanded, expErr := expandVars(raw)
	if expErr != nil {
		return refs, nil
	}
	var m map[string]any
	if err := yaml.Unmarshal(expanded, &m); err != nil {
		return refs, nil
	}
	dir := filepath.Dir(path)
	for _, inc := range includePaths(m["include"]) {
		if !filepath.IsAbs(inc) {
			inc = filepath.Join(dir, inc)
		}
		files, err := expandInclude(inc)
		if err != nil {
			continue // a broken include is reported by the loader, not here
		}
		for _, f := range files {
			sub, err := scanVars(f, visiting)
			if err != nil {
				continue
			}
			refs = append(refs, sub...)
		}
	}
	return refs, nil
}

// isEnvName reports whether s has the shape of an environment variable name.
// Used to tell a real reference from prose that happens to contain ${…}.
func isEnvName(s string) bool {
	for i, r := range s {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r == '_':
		case r >= '0' && r <= '9' && i > 0: // a digit may not lead
		default:
			return false
		}
	}
	return s != ""
}

// refsIn extracts the references from one file's text, mirroring expandVars's
// grammar — including the `$${` escape, which is a literal and not a reference.
func refsIn(text, file string) []VarRef {
	text = strings.ReplaceAll(text, "$${", "\x00")

	var out []VarRef
	seen := map[string]bool{}
	for _, m := range varPattern.FindAllStringSubmatch(text, -1) {
		inner := m[1]
		ref := VarRef{File: file}
		switch {
		case strings.HasPrefix(inner, "file:"):
			ref.Kind = VarFile
			ref.Name = strings.TrimPrefix(inner, "file:")
			if _, err := os.Stat(ref.Name); err == nil {
				ref.Resolved = true
			}
		case strings.Contains(inner, ":-"):
			name, _, _ := strings.Cut(inner, ":-")
			ref.Kind, ref.Name, ref.HasDefault = VarEnv, name, true
			ref.Resolved = os.Getenv(name) != ""
		default:
			ref.Kind, ref.Name = VarEnv, inner
			ref.Resolved = os.Getenv(inner) != ""
		}
		if ref.Name == "" {
			continue
		}
		// Prose, not a reference. checkfleet.example.yml has the comment
		// "…comes from the environment via ${...} interpolation", and without
		// this the scan reports a missing variable literally named "...".
		// expandVars would replace it with "" too, but inside a comment that is
		// harmless — reporting it as a problem is not.
		if ref.Kind == VarEnv && !isEnvName(ref.Name) {
			continue
		}
		// One entry per name per file: the same secret referenced by three
		// targets is one thing to fix, not three.
		key := string(ref.Kind) + "\x00" + ref.Name
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, ref)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
