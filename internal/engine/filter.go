package engine

import (
	"path"
	"strings"
)

// FilterOptions narrows a set of findings for output.
type FilterOptions struct {
	Only        map[string]bool // check names to keep; empty = all
	MinSeverity Status          // keep findings at or above this severity; "" = all
	TargetGlob  string          // path.Match glob on the target; "" = all
}

// Filter returns the findings that pass every set criterion, preserving order.
func Filter(findings []Finding, o FilterOptions) []Finding {
	minSev, hasMin := severity[o.MinSeverity], o.MinSeverity != ""
	var out []Finding
	for _, f := range findings {
		if len(o.Only) > 0 && !o.Only[f.Check] {
			continue
		}
		if hasMin && severity[f.Status] < minSev {
			continue
		}
		if o.TargetGlob != "" {
			if ok, _ := path.Match(o.TargetGlob, f.Target); !ok {
				continue
			}
		}
		out = append(out, f)
	}
	return out
}

// ParseStatus maps a case-insensitive name to a Status ("" input → ("", true)).
func ParseStatus(s string) (Status, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return "", true
	case "ok":
		return OK, true
	case "warn":
		return WARN, true
	case "bad":
		return BAD, true
	case "error":
		return ERROR, true
	default:
		return "", false
	}
}
