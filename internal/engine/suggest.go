package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Problem is one validation finding with, where possible, the fix.
//
// The distinction from a bare message: "unknown module postgress" tells you
// something is wrong, "did you mean postgres?" tells you what to type. The
// second is the difference between a validator and a useful one.
type Problem struct {
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
	// Advisory marks a problem that depends on **this machine** rather than on
	// the config: an unset ${VAR}, an unreadable secret file. Worth saying, but
	// it must not fail `validate` — that command is documented for pre-commit
	// hooks, and a laptop legitimately doesn't have production secrets exported.
	// `doctor` is the command that treats the environment as its subject.
	Advisory bool `json:"advisory,omitempty"`
}

// Blocking reports whether any problem is a real config defect, as opposed to
// something about the machine it was inspected on.
func Blocking(problems []Problem) bool {
	for _, p := range problems {
		if !p.Advisory {
			return true
		}
	}
	return false
}

// String renders a problem as one line, with the suggestion appended.
func (p Problem) String() string {
	if p.Suggestion == "" {
		return p.Message
	}
	return p.Message + " → " + p.Suggestion
}

// Inspect validates a config and returns problems with actionable suggestions.
//
// It looks at both the raw file and the parsed config, because the two see
// different classes of mistake. The parsed Config cannot express a **misspelled
// key**: YAML unmarshalling silently drops anything that doesn't match a field,
// so `postgress:` under `checks` means the module never runs, `validate` used to
// report only "no module configured", and nothing ever pointed at the typo.
//
// cfg may be nil (the config failed to load); the raw-level checks still run.
func Inspect(path string, cfg *Config) []Problem {
	var out []Problem
	out = append(out, UnknownKeys(path)...)
	out = append(out, missingVars(path)...)

	if cfg == nil {
		return out
	}
	for _, msg := range Validate(cfg) {
		out = append(out, Problem{Message: msg, Suggestion: suggestFor(msg)})
	}
	return out
}

// keysOutsideTheStruct are valid top-level keys that are NOT Config fields
// because the loader consumes them from the raw map before unmarshalling.
// Without this list, a config using the documented `include:` feature (CF-115)
// would be told its include is an unknown key — a false positive on a correct
// config, which is worse than the missing check it replaces.
var keysOutsideTheStruct = []string{"include"}

// topLevelKeys and moduleKeys are the valid YAML keys, read off the structs so
// they can never drift from what the config actually accepts.
func topLevelKeys() []string {
	return append(yamlKeysOf(reflect.TypeOf(Config{})), keysOutsideTheStruct...)
}
func moduleKeys() []string { return yamlKeysOf(reflect.TypeOf(ChecksConfig{})) }

func yamlKeysOf(t reflect.Type) []string {
	var out []string
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("yaml")
		if tag == "" || tag == "-" {
			continue
		}
		if name, _, _ := strings.Cut(tag, ","); name != "" {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// UnknownKeys reports the keys a config file — and the files it includes —
// declares that YAML silently ignores, with the closest valid name when there is
// one.
//
// It is exported because `check` warns about them too (CF-154), not just the
// diagnostic commands. This is the one config mistake where silence is worse
// than noise: an ignored key means the module never runs, and the run then
// reports a *healthy* fleet because it checked nothing.
//
// A missing or unparseable file yields nothing: those are the loader's errors to
// report, and this must never become a second, worse-worded copy of them.
func UnknownKeys(path string) []Problem { return unknownKeys(path, map[string]bool{}, true) }

// unknownKeys walks path and its include chain. root marks the file the caller
// asked about, whose problems read better without a filename in them.
func unknownKeys(path string, visiting map[string]bool, root bool) []Problem {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	if visiting[abs] {
		return nil // include cycle: the loader reports it
	}
	visiting[abs] = true
	defer delete(visiting, abs)

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	// Interpolate first so a ${...} inside a key doesn't read as unknown; a
	// failure here is not ours to report (doctor and the loader both do).
	if expanded, err := expandVars(raw); err == nil {
		raw = expanded
	}
	var m map[string]any
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return nil // unparseable: the loader reports it
	}

	// An included file's problems name the file: the main config looks fine, so
	// without it the reader has nowhere to go looking.
	in := ""
	if !root {
		in = " of " + filepath.Base(path)
	}

	var out []Problem
	out = append(out, unknownIn(m, topLevelKeys(), "top level"+in)...)
	if checks, ok := m["checks"].(map[string]any); ok {
		out = append(out, unknownIn(checks, moduleKeys(), "`checks`"+in)...)
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
			out = append(out, unknownKeys(f, visiting, false)...)
		}
	}
	return out
}

func unknownIn(m map[string]any, valid []string, where string) []Problem {
	validSet := make(map[string]bool, len(valid))
	for _, v := range valid {
		validSet[v] = true
	}

	var keys []string
	for k := range m {
		if !validSet[strings.ToLower(k)] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	out := make([]Problem, 0, len(keys))
	for _, k := range keys {
		p := Problem{Message: fmt.Sprintf("unknown key %q at %s — it is ignored, so nothing you configured under it runs", k, where)}
		if near := closest(k, valid); near != "" {
			p.Suggestion = fmt.Sprintf("did you mean %q?", near)
		}
		out = append(out, p)
	}
	return out
}

// missingVars reports ${VAR} references that expand to nothing.
func missingVars(path string) []Problem {
	refs, err := ScanVars(path)
	if err != nil {
		return nil
	}
	var out []Problem
	for _, r := range refs {
		if !r.Missing() {
			continue
		}
		if r.Kind == VarFile {
			out = append(out, Problem{
				Message:    fmt.Sprintf("secret file %q cannot be read", r.Name),
				Suggestion: "create the file, or point ${file:...} somewhere readable",
				Advisory:   true,
			})
			continue
		}
		out = append(out, Problem{
			Message: fmt.Sprintf("environment variable %s is not set, so it expands to an empty value", r.Name),
			Suggestion: fmt.Sprintf("export %s=... before running, or write ${%s:-default} to make the fallback explicit",
				r.Name, r.Name),
			Advisory: true,
		})
	}
	return out
}

// suggestFor attaches a fix to the messages Validate produces. Matching on the
// message text is deliberately conservative: a missing suggestion is fine, a
// wrong one is worse than none.
func suggestFor(msg string) string {
	switch {
	case strings.Contains(msg, "no module configured"):
		return "scaffold one with: checkfleet init --modules certs,http"
	case strings.HasSuffix(msg, "no target"), strings.Contains(msg, "no target or ansible_inventory"):
		module, _, _ := strings.Cut(msg, ":")
		return fmt.Sprintf("add targets under `checks.%s.targets`, or run: checkfleet explain %s", module, module)
	case strings.Contains(msg, "has no url"):
		return "every http target needs a `url:`"
	case strings.Contains(msg, "has no dsn"), strings.Contains(msg, "has no address"):
		return "give the target an address; credentials belong in ${ENV}, never inline"
	case strings.Contains(msg, "should be >="), strings.Contains(msg, ">"):
		return "the warn threshold must trigger before the crit one — check the two values are not swapped"
	}
	return ""
}

// closest returns the candidate within a small edit distance of s, or "".
//
// The threshold scales with the word: one edit for a short name, up to three for
// a long one. Too generous and every unknown key gets a confidently wrong
// suggestion ("did you mean s3?" for "smtpp").
func closest(s string, candidates []string) string {
	s = strings.ToLower(s)

	// A prefix is not a typo by edit distance ("elastic" is six edits from
	// "elasticsearch") but it is almost always the intended key — someone typed
	// the short name. Require three characters so "s" doesn't match "smtp".
	if len(s) >= 3 {
		for _, c := range candidates {
			if c != s && strings.HasPrefix(c, s) {
				return c
			}
		}
	}

	best, bestDist := "", 1<<31-1
	max := 1 + len(s)/4
	if max > 3 {
		max = 3
	}
	for _, c := range candidates {
		d := editDistance(s, c)
		if d < bestDist {
			best, bestDist = c, d
		}
	}
	if bestDist > max {
		return ""
	}
	return best
}

// editDistance is Levenshtein, iterative with two rows (zero-dep, no matrix).
func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 {
		return len(rb)
	}
	prev := make([]int, len(rb)+1)
	curr := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		curr[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			curr[j] = min3(curr[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(rb)]
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}
