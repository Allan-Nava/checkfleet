package insight

import (
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/history"
)

func num(v float64) *float64 { return &v }

func records() []history.Record {
	return []history.Record{
		// Deliberately out of order: SeriesFrom must not trust the input order.
		{Unix: 200, Entries: []history.Entry{
			{Check: "postgres", Target: "db-01", Status: "WARN", Value: num(82), Unit: "%"},
			{Check: "certs", Target: "a:443", Status: "OK"},
		}},
		{Unix: 100, Entries: []history.Entry{
			{Check: "postgres", Target: "db-01", Status: "OK", Value: num(80), Unit: "%"},
			{Check: "certs", Target: "a:443", Status: "OK"},
		}},
	}
}

func TestSeriesFromOrdersPointsAndKeepsUnit(t *testing.T) {
	s := SeriesFrom(records())
	if len(s) != 1 {
		t.Fatalf("got %d series, want 1 (only postgres carries a metric)", len(s))
	}
	if s[0].Check != "postgres" || s[0].Target != "db-01" || s[0].Unit != "%" {
		t.Errorf("series identity/unit wrong: %+v", s[0])
	}
	if got := []int64{s[0].Points[0].Unix, s[0].Points[1].Unix}; got[0] != 100 || got[1] != 200 {
		t.Errorf("points not chronological: %v", got)
	}
}

func TestSeriesFromSkipsFindingsWithoutAMetric(t *testing.T) {
	for _, s := range SeriesFrom(records()) {
		if s.Check == "certs" {
			t.Error("a status-only finding must not produce a metric series")
		}
	}
}

func TestStatusSeriesKeepsEveryFinding(t *testing.T) {
	s := StatusSeriesFrom(records())
	if len(s) != 2 {
		t.Fatalf("got %d status series, want 2", len(s))
	}
	// Sorted by key, so certs comes first.
	if s[0].Check != "certs" {
		t.Errorf("series are not key-sorted: %s first", s[0].Check)
	}
	if s[1].Points[0].Status != "OK" || s[1].Points[1].Status != "WARN" {
		t.Errorf("status points not chronological: %+v", s[1].Points)
	}
}

func TestSeriesOrderIsStable(t *testing.T) {
	first := SeriesFrom(records())
	for i := 0; i < 5; i++ {
		next := SeriesFrom(records())
		if len(next) != len(first) || next[0].Key() != first[0].Key() {
			t.Fatal("series order is not stable across runs — map iteration leaked out")
		}
	}
}
