package engine

import "testing"

func TestDiffStatus(t *testing.T) {
	prev := map[string]Status{"a": OK, "b": BAD, "c": WARN, "e": BAD}
	curr := map[string]Status{"a": BAD, "b": OK, "c": ERROR, "d": BAD, "e": WARN}

	got := map[string]Change{}
	for _, ch := range DiffStatus(prev, curr) {
		got[ch.Key] = ch
	}

	cases := map[string]ChangeKind{
		"a": ChangeNew,      // OK -> BAD
		"b": ChangeResolved, // BAD -> OK
		"c": ChangeWorsened, // WARN -> ERROR
		"d": ChangeNew,      // absent -> BAD
		"e": ChangeImproved, // BAD -> WARN
	}
	for k, want := range cases {
		if got[k].Kind != want {
			t.Errorf("%s: want %s, got %s (%+v)", k, want, got[k].Kind, got[k])
		}
	}
	if len(got) != len(cases) {
		t.Errorf("unexpected changes: %v", got)
	}
}

func TestDiffStatusNoChange(t *testing.T) {
	m := map[string]Status{"a": OK, "b": BAD}
	if d := DiffStatus(m, m); len(d) != 0 {
		t.Errorf("identical runs should have no diff, got %v", d)
	}
}
