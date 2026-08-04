package engine

import "path"

// ApplyRunbooks attaches operator hints from the config to the findings that
// need attention (CF-124): a runbook URL and a short remediation note, so an
// operator reading a BAD does not have to go and find the procedure.
//
// Only findings above OK are annotated — a hint on a green result is noise in
// every output, and there is nothing to do about it. Rules are read in order
// and the first non-empty value wins per field, so a specific rule and a
// catch-all can each contribute what the other leaves out. A finding that
// already carries a hint keeps it.
func ApplyRunbooks(findings []Finding, rules []RunbookRule) []Finding {
	if len(rules) == 0 {
		return findings
	}
	out := findings[:0:0] // new slice, keep input untouched
	for _, f := range findings {
		if f.Status == OK {
			out = append(out, f)
			continue
		}
		for _, r := range rules {
			if f.Runbook != "" && f.Remediation != "" {
				break
			}
			if !runbookMatches(r, f) {
				continue
			}
			if f.Runbook == "" {
				f.Runbook = r.Runbook
			}
			if f.Remediation == "" {
				f.Remediation = r.Remediation
			}
		}
		out = append(out, f)
	}
	return out
}

// runbookMatches reports whether the rule's check/target globs cover f.
func runbookMatches(r RunbookRule, f Finding) bool {
	if r.Check != "" {
		if ok, _ := path.Match(r.Check, f.Check); !ok {
			return false
		}
	}
	if r.Target != "" {
		if ok, _ := path.Match(r.Target, f.Target); !ok {
			return false
		}
	}
	return true
}
