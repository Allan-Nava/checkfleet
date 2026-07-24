package history

import (
	"path/filepath"
	"testing"
)

func rec(unix int64, entries ...Entry) Record { return Record{Unix: unix, Entries: entries} }
func e(check, target, status string) Entry {
	return Entry{Check: check, Target: target, Status: status}
}

func TestAppendAndRecent(t *testing.T) {
	s := Open(filepath.Join(t.TempDir(), "h.jsonl"))
	for i := int64(1); i <= 5; i++ {
		if err := s.Append(rec(i, e("http", "a", "OK"))); err != nil {
			t.Fatal(err)
		}
	}
	all, err := s.Recent(0)
	if err != nil || len(all) != 5 {
		t.Fatalf("Recent(0): want 5, got %d (%v)", len(all), err)
	}
	last3, _ := s.Recent(3)
	if len(last3) != 3 || last3[0].Unix != 3 || last3[2].Unix != 5 {
		t.Errorf("Recent(3) sbagliato: %+v", last3)
	}
}

func TestRecentMissingFile(t *testing.T) {
	s := Open(filepath.Join(t.TempDir(), "nope.jsonl"))
	if r, err := s.Recent(10); err != nil || r != nil {
		t.Errorf("missing file: want nil,nil; got %v,%v", r, err)
	}
}

func TestFlapsCountsTransitions(t *testing.T) {
	records := []Record{
		rec(1, e("http", "a", "OK"), e("http", "b", "OK")),
		rec(2, e("http", "a", "BAD"), e("http", "b", "OK")), // a: OK→BAD (1)
		rec(3, e("http", "a", "OK"), e("http", "b", "OK")),  // a: BAD→OK (2)
		rec(4, e("http", "a", "BAD"), e("http", "b", "OK")), // a: OK→BAD (3)
	}
	flaps := Flaps(records, 3)
	if len(flaps) != 1 || flaps[0].Key != "http/a" || flaps[0].Changes != 3 || flaps[0].Last != "BAD" {
		t.Errorf("flap expected http/a with 3 changes last BAD, got %+v", flaps)
	}
}

func TestFlapsBelowThreshold(t *testing.T) {
	records := []Record{
		rec(1, e("dns", "x", "OK")),
		rec(2, e("dns", "x", "WARN")), // 1 cambio
	}
	if f := Flaps(records, 3); len(f) != 0 {
		t.Errorf("below threshold: want 0 flaps, got %+v", f)
	}
}
