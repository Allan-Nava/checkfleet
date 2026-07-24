// Package history persists a compact snapshot of each run to a JSONL file
// (one record per line, zero dependencies) and derives flapping/persistence
// signals from the recent records. It is the lightweight alternative to a
// database: enough for "has this been flapping / how long has it been BAD".
package history

import (
	"bufio"
	"encoding/json"
	"os"
)

// Entry is one finding's identity+status in a run.
type Entry struct {
	Check  string `json:"c"`
	Target string `json:"g"`
	Status string `json:"s"`
}

// Record is one run's snapshot.
type Record struct {
	Unix    int64   `json:"t"`
	Entries []Entry `json:"f"`
}

// Store is an append-only JSONL history file.
type Store struct{ path string }

func Open(path string) *Store { return &Store{path: path} }

// Append writes one record as a JSON line.
func (s *Store) Append(r Record) error {
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
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var r Record
		if err := json.Unmarshal(line, &r); err == nil {
			records = append(records, r)
		}
	}
	if err := sc.Err(); err != nil {
		return records, err
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
		last     string
		changes  int
		seen     bool
		firstIdx int
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
