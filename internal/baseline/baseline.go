// Package baseline records the findings a fleet already has, so a pipeline can
// gate on *new* problems only.
//
// The problem it solves: adopting checkfleet on a fleet that is already dirty.
// With a plain gate the first run is red and stays red, so the gate gets
// disabled and stops protecting anything. A baseline freezes the known debt and
// lets the build fail only on what appeared — or got worse — since.
package baseline

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// key identifies one check/target pair. Same encoding as the --diff view, so
// the two features agree on what "the same finding" means.
func key(check, target string) string { return check + "\t" + target }

// Entry is one recorded check/target status.
type Entry struct {
	Check  string `json:"check"`
	Target string `json:"target"`
	Status string `json:"status"`
}

// File is the on-disk baseline.
type File struct {
	Version  int       `json:"version"`
	Recorded time.Time `json:"recorded"`
	Entries  []Entry   `json:"entries"`
}

// currentVersion of the file format. Bump it only for a breaking change; Load
// rejects anything it does not understand rather than guessing.
const currentVersion = 1

// Save writes the findings as a baseline. Every finding is recorded, OK
// included: "this target was fine" is what makes a later regression detectable.
func Save(path string, findings []engine.Finding, now time.Time) error {
	f := File{Version: currentVersion, Recorded: now, Entries: make([]Entry, 0, len(findings))}
	for _, x := range findings {
		f.Entries = append(f.Entries, Entry{Check: x.Check, Target: x.Target, Status: string(x.Status)})
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// Load reads a baseline file.
func Load(path string) (File, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return File{}, err
	}
	var f File
	if err := json.Unmarshal(b, &f); err != nil {
		return File{}, fmt.Errorf("parsing baseline %s: %w", path, err)
	}
	if f.Version != currentVersion {
		return File{}, fmt.Errorf("baseline %s has version %d, this checkfleet understands %d (re-record it with --write-baseline)",
			path, f.Version, currentVersion)
	}
	return f, nil
}

// StatusMap indexes the baseline by check/target.
func (f File) StatusMap() map[string]engine.Status {
	m := make(map[string]engine.Status, len(f.Entries))
	for _, e := range f.Entries {
		m[key(e.Check, e.Target)] = engine.Status(e.Status)
	}
	return m
}

// NewOrWorse returns the findings that the baseline does not already excuse:
// a check/target it never saw, or one whose status has got worse since.
//
// A target that was BAD and is still BAD is *not* returned — that is the known
// debt the baseline exists to tolerate. One that was WARN and is now BAD is,
// because a regression on a known-imperfect target is still a regression.
func NewOrWorse(current []engine.Finding, f File) []engine.Finding {
	prev := f.StatusMap()
	curr := make(map[string]engine.Status, len(current))
	byKey := make(map[string]engine.Finding, len(current))
	for _, x := range current {
		k := key(x.Check, x.Target)
		curr[k] = x.Status
		byKey[k] = x
	}

	var out []engine.Finding
	// DiffStatus already classifies the transitions (and treats an absent key as
	// OK, so a target missing from the baseline reads as "new").
	for _, c := range engine.DiffStatus(prev, curr) {
		if c.Kind == engine.ChangeNew || c.Kind == engine.ChangeWorsened {
			out = append(out, byKey[c.Key])
		}
	}
	return out
}
