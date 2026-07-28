package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// docPath is the compatibility contract these formats are declared in. A key
// that changes here must change there too — that is what these tests enforce.
const docPath = "../../docs/compatibility.md"

// keysOf returns the top-level JSON object keys of v.
func keysOf(t *testing.T, v any) []string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func TestRecordJSONKeys(t *testing.T) {
	got := strings.Join(keysOf(t, Record{Unix: 1, Schema: SchemaVersion}), ",")
	// Deliberately spelled out: these single letters are on disk in every user's
	// history file, and renaming one silently breaks --diff and the GUI charts.
	if want := "f,sv,t"; got != want {
		t.Errorf("Record keys = %q, want %q — see %s before changing the format", got, want, docPath)
	}
}

func TestEntryJSONKeys(t *testing.T) {
	v := 1.5
	got := strings.Join(keysOf(t, Entry{Check: "http", Target: "a", Status: "OK", Value: &v, Unit: "ms"}), ",")
	if want := "c,g,s,u,v"; got != want {
		t.Errorf("Entry keys = %q, want %q — see %s before changing the format", got, want, docPath)
	}
}

func TestAppendStampsSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "h.jsonl")
	s := Open(path)
	// Callers construct Record without Schema (the desktop does): Append stamps it.
	if err := s.Append(rec(1, e("http", "a", "OK"))); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"sv":1`) {
		t.Errorf("appended line missing schema version: %s", raw)
	}
	recs, err := s.Recent(0)
	if err != nil || len(recs) != 1 || recs[0].Schema != SchemaVersion {
		t.Errorf("Recent: want one record with schema %d, got %+v (%v)", SchemaVersion, recs, err)
	}
}

func TestRecentReadsLegacyRecordWithoutSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "h.jsonl")
	// A file written before CF-153: no "sv" at all. It must keep working — users
	// have these on disk and --fail-on-new / --diff read them.
	legacy := `{"t":42,"f":[{"c":"http","g":"a","s":"BAD"}]}` + "\n"
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	recs, err := Open(path).Recent(0)
	if err != nil {
		t.Fatalf("legacy record: unexpected error %v", err)
	}
	if len(recs) != 1 || recs[0].Unix != 42 || recs[0].Schema != 1 {
		t.Fatalf("legacy record: want one v1 record at t=42, got %+v", recs)
	}
	if len(recs[0].Entries) != 1 || recs[0].Entries[0].Status != "BAD" {
		t.Errorf("legacy entries lost: %+v", recs[0].Entries)
	}
}

func TestRecentSkipsNewerSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "h.jsonl")
	// A newer checkfleet wrote a format this binary does not understand. Reading
	// it as if it were v1 would produce a confidently wrong diff, so it is
	// skipped and reported instead of guessed.
	lines := `{"t":1,"sv":1,"f":[{"c":"http","g":"a","s":"OK"}]}` + "\n" +
		`{"t":2,"sv":99,"f":[{"c":"http","g":"a","s":"OK"}]}` + "\n"
	if err := os.WriteFile(path, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	recs, err := Open(path).Recent(0)
	if err == nil {
		t.Fatal("newer schema: want an error naming the version, got nil")
	}
	if !strings.Contains(err.Error(), "99") {
		t.Errorf("error should name the unknown version, got %v", err)
	}
	if len(recs) != 1 || recs[0].Unix != 1 {
		t.Errorf("readable records should still be returned, got %+v", recs)
	}
}

// TestKeysAreDocumented is the anti-drift gate: the on-disk keys must be listed
// in the compatibility contract, so a format change cannot land without the doc
// being updated in the same commit.
func TestKeysAreDocumented(t *testing.T) {
	doc, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"t", "f", "sv", "c", "g", "s", "v", "u"} {
		if !strings.Contains(string(doc), "`"+key+"`") {
			t.Errorf("history key %q is not documented in %s", key, docPath)
		}
	}
}
