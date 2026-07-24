package engine

import "path"

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
	switch s {
	case "":
		return "", true
	case "ok", "OK":
		return OK, true
	case "warn", "WARN":
		return WARN, true
	case "bad", "BAD":
		return BAD, true
	case "error", "ERROR":
		return ERROR, true
	default:
		return "", false
	}
}
