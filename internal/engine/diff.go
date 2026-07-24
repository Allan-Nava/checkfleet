package engine

import "sort"

// ChangeKind classifies how a check/target's status moved between two runs.
type ChangeKind string

const (
	ChangeNew      ChangeKind = "new"      // was OK/absent, now non-OK
	ChangeWorsened ChangeKind = "worsened" // non-OK, moved to a worse status
	ChangeImproved ChangeKind = "improved" // non-OK, moved to a better (still non-OK) status
	ChangeResolved ChangeKind = "resolved" // was non-OK, now OK/absent
)

// Change is one status transition for a key between two runs.
type Change struct {
	Key  string
	From Status
	To   Status
	Kind ChangeKind
}

// DiffStatus compares two key→status maps (previous vs current) and returns the
// changes, sorted by key. Missing keys count as OK on their side, so a target
// that appears/disappears reads as new/resolved.
func DiffStatus(prev, curr map[string]Status) []Change {
	keys := map[string]struct{}{}
	for k := range prev {
		keys[k] = struct{}{}
	}
	for k := range curr {
		keys[k] = struct{}{}
	}

	var out []Change
	for k := range keys {
		p, c := prev[k], curr[k]
		if p == "" {
			p = OK
		}
		if c == "" {
			c = OK
		}
		if p == c {
			continue
		}
		ch := Change{Key: k, From: p, To: c}
		if severity[c] > severity[p] {
			if p == OK {
				ch.Kind = ChangeNew
			} else {
				ch.Kind = ChangeWorsened
			}
		} else {
			if c == OK {
				ch.Kind = ChangeResolved
			} else {
				ch.Kind = ChangeImproved
			}
		}
		out = append(out, ch)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}
