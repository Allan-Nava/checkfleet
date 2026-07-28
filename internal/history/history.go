// Package history persists a compact snapshot of each run to a JSONL file
// (one record per line, zero dependencies) and derives flapping/persistence
// signals from the recent records. It is the lightweight alternative to a
// database: enough for "has this been flapping / how long has it been BAD".
package history

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

// SchemaVersion is the on-disk format of a history record, stamped on every
// line as "sv" (CF-153). It exists so the format can evolve without silently
// corrupting the files users already have: `--diff` and `--fail-on-new` read
// records written by earlier runs, and a reader that guesses the layout of a
// record it does not understand produces a confidently wrong diff.
//
// Bump it only for a breaking change to Record/Entry. Records written before
// this field existed read as version 1 (see Recent).
const SchemaVersion = 1

// Entry is one finding's identity+status in a run. Value/Unit are the optional
// scalar metric (CF-91), persisted so the GUI can chart it over time.
type Entry struct {
	Check  string   `json:"c"`
	Target string   `json:"g"`
	Status string   `json:"s"`
	Value  *float64 `json:"v,omitempty"`
	Unit   string   `json:"u,omitempty"`
}

// Record is one run's snapshot. Schema is the format version (SchemaVersion);
// Append stamps it, so callers need not set it.
type Record struct {
	Unix    int64   `json:"t"`
	Entries []Entry `json:"f"`
	Schema  int     `json:"sv"`
}

// Store is an append-only JSONL history file.
type Store struct{ path string }

func Open(path string) *Store { return &Store{path: path} }

// Append writes one record as a JSON line, stamped with SchemaVersion.
func (s *Store) Append(r Record) error {
	if r.Schema == 0 {
		r.Schema = SchemaVersion
	}
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	line, err := json.Marshal(r)
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	return err
}

// Recent returns the last n records in chronological order (all if n<=0). A
// missing file is not an error — it returns no records.
//
// Records written before SchemaVersion existed read as version 1. Records
// stamped with a *newer* version are skipped and reported: they were written by
// a checkfleet that knows a layout this one does not, and reading them anyway
// would yield a wrong diff rather than an obvious failure. The readable records
// are still returned alongside the error.
func (s *Store) Recent(n int) ([]Record, error) {
	f, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var records []Record
	newest := 0 // highest unknown schema version seen, if any
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var r Record
		if err := json.Unmarshal(line, &r); err != nil {
			continue
		}
		if r.Schema == 0 {
			r.Schema = 1 // written before the field existed
		}
		if r.Schema > SchemaVersion {
			if r.Schema > newest {
				newest = r.Schema
			}
			continue
		}
		records = append(records, r)
	}
	if err := sc.Err(); err != nil {
		return records, err
	}
	if newest > 0 {
		return records, fmt.Errorf("%s contains records with schema version %d, this checkfleet understands %d: they were skipped (upgrade checkfleet, or start a new history file)",
			s.path, newest, SchemaVersion)
	}
	if n > 0 && len(records) > n {
		records = records[len(records)-n:]
	}
	return records, nil
}

// Flap is a key whose status changed repeatedly across the window.
type Flap struct {
	Key     string
	Changes int
	Last    string
}

// Flaps counts status transitions per key across records (chronological) and
// returns keys with at least minChanges transitions. Key is "check/target".
func Flaps(records []Record, minChanges int) []Flap {
	type seq struct {
		last    string
		changes int
		seen    bool
	}
	state := map[string]*seq{}
	order := []string{}
	for _, r := range records {
		for _, e := range r.Entries {
			key := e.Check + "/" + e.Target
			st, ok := state[key]
			if !ok {
				st = &seq{}
				state[key] = st
				order = append(order, key)
			}
			if st.seen && e.Status != st.last {
				st.changes++
			}
			st.last = e.Status
			st.seen = true
		}
	}
	var flaps []Flap
	for _, key := range order {
		st := state[key]
		if st.changes >= minChanges {
			flaps = append(flaps, Flap{Key: key, Changes: st.changes, Last: st.last})
		}
	}
	return flaps
}
