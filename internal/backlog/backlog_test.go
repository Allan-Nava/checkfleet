package backlog

import "testing"

const sample = `# Backlog — checkfleet

Sorgente unica dei todo.

## M1 — Rete & delivery (~v0.2) — il cuore rete & delivery

- [x] **CF-1 — Modulo ` + "`nats`" + `**: preflight/health cluster NATS. _(v0.2.0)_
- [x] **CF-2 — Modulo ` + "`haproxy`" + `**: backend/server DOWN via CSV stats.

## M2 — Data layer (~v0.3)

- [ ] **CF-4 — Modulo ` + "`patroni`" + `**: leader per cluster, repliche in lag.
- [ ] **CF-11 — Modulo ` + "`postgres`" + `**: reachability, replica lag.

## Rilascio

- [ ] **CF-10 — Docs sito** (README ricco con GIF).
`

func TestParseCountAndOrder(t *testing.T) {
	items := Parse(sample)
	wantIDs := []string{"CF-1", "CF-2", "CF-4", "CF-11", "CF-10"}
	if len(items) != len(wantIDs) {
		t.Fatalf("attesi %d item, avuti %d: %+v", len(wantIDs), len(items), items)
	}
	for i, id := range wantIDs {
		if items[i].ID != id {
			t.Errorf("position %d: want %s, got %s", i, id, items[i].ID)
		}
	}
}

func TestParseFields(t *testing.T) {
	byID := map[string]Item{}
	for _, it := range Parse(sample) {
		byID[it.ID] = it
	}

	cf1 := byID["CF-1"]
	if !cf1.Done {
		t.Errorf("CF-1 dovrebbe essere done")
	}
	if cf1.Milestone != "M1 — Rete & delivery" {
		t.Errorf("CF-1 milestone: %q", cf1.Milestone)
	}
	if cf1.Title != "CF-1 — Modulo nats" { // backtick rimossi
		t.Errorf("CF-1 title: %q", cf1.Title)
	}

	cf4 := byID["CF-4"]
	if cf4.Done {
		t.Errorf("CF-4 dovrebbe essere aperto")
	}
	if cf4.Milestone != "M2 — Data layer" {
		t.Errorf("CF-4 milestone: %q", cf4.Milestone)
	}
	if cf4.Description == "" {
		t.Errorf("CF-4 dovrebbe avere una descrizione")
	}

	// "Rilascio" non ha una parte "(~v)": va tenuta intera.
	if byID["CF-10"].Milestone != "Rilascio" {
		t.Errorf("CF-10 milestone: %q", byID["CF-10"].Milestone)
	}
}

func TestParseIgnoresNonItemLines(t *testing.T) {
	md := "## M5 — App desktop\n\nStack scelto: Wails, testo di paragrafo.\n\n- [ ] **CF-15 — Scaffold**: crea il progetto.\n"
	items := Parse(md)
	if len(items) != 1 || items[0].ID != "CF-15" {
		t.Fatalf("il paragrafo non-item dev'essere ignorato, avuti: %+v", items)
	}
}
