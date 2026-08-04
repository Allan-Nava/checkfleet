package insight

import (
	"strings"
	"testing"
	"time"
)

// series builds one named status series from a list of statuses.
func series(check, target string, ss ...string) StatusSeries {
	s := StatusSeries{Check: check, Target: target}
	for i, v := range ss {
		s.Points = append(s.Points, StatusPoint{Unix: t0.Add(time.Duration(i) * time.Hour).Unix(), Status: v})
	}
	return s
}

func TestDigestSortsMovementsIntoTheFourBuckets(t *testing.T) {
	d := Compare([]StatusSeries{
		series("http", "new", "OK", "OK", "BAD"),
		series("http", "fixed", "BAD", "BAD", "OK"),
		series("http", "worse", "WARN", "WARN", "BAD"),
		series("http", "better", "BAD", "BAD", "WARN"),
		series("http", "steady", "OK", "OK", "OK"),
	}, 99)
	if len(d.New) != 1 || d.New[0].Target != "new" {
		t.Errorf("New = %+v", d.New)
	}
	if len(d.Resolved) != 1 || d.Resolved[0].Target != "fixed" {
		t.Errorf("Resolved = %+v", d.Resolved)
	}
	if len(d.Degraded) != 1 || d.Degraded[0].Target != "worse" {
		t.Errorf("Degraded = %+v", d.Degraded)
	}
	if len(d.Improved) != 1 || d.Improved[0].Target != "better" {
		t.Errorf("Improved = %+v", d.Improved)
	}
}

func TestSteadyFleetSaysSoInOneLine(t *testing.T) {
	d := Compare([]StatusSeries{series("http", "a", "OK", "OK", "OK")}, 99)
	if !d.Empty() {
		t.Fatalf("want an empty digest, got %+v", d)
	}
	out := Narrate(d)
	if strings.Count(out, "\n") > 1 {
		t.Errorf("an empty digest must not print four empty headings:\n%s", out)
	}
	if !strings.Contains(out, "Nothing changed") {
		t.Errorf("unexpected wording: %q", out)
	}
}

// TestErrorToBadIsADegradation checks the severity order: ERROR ranks below BAD
// because "could not measure" is weaker news than "measured and it is broken".
func TestErrorToBadIsADegradation(t *testing.T) {
	d := Compare([]StatusSeries{series("http", "a", "ERROR", "BAD")}, 99)
	if len(d.Degraded) != 1 {
		t.Errorf("ERROR → BAD should be a degradation: %+v", d)
	}
	back := Compare([]StatusSeries{series("http", "a", "BAD", "ERROR")}, 99)
	if len(back.Improved) != 1 {
		t.Errorf("BAD → ERROR should be an improvement in this ordering: %+v", back)
	}
}

// TestUnknownStatusNeverFabricatesARegression: a record written by a newer
// build must not read as a problem just because this build cannot rank it.
func TestUnknownStatusNeverFabricatesARegression(t *testing.T) {
	d := Compare([]StatusSeries{series("http", "a", "OK", "FUTURE")}, 99)
	if len(d.New) != 0 || len(d.Degraded) != 0 {
		t.Errorf("an unrankable status must not become a problem: %+v", d)
	}
}

func TestFlappingIsReportedAlongsideTheMovement(t *testing.T) {
	d := Compare([]StatusSeries{series("http", "a", "OK", "BAD", "OK", "BAD")}, 3)
	if len(d.Flapping) != 1 {
		t.Fatalf("want the oscillating target flagged: %+v", d)
	}
	if !strings.Contains(Narrate(d), "Flapping:") {
		t.Error("the narrative should carry a flapping section")
	}
}

func TestNarrativeLeadsWithTheCounts(t *testing.T) {
	d := Compare([]StatusSeries{
		series("http", "a", "OK", "BAD"),
		series("http", "b", "BAD", "OK"),
	}, 99)
	first := strings.SplitN(Narrate(d), "\n", 2)[0]
	if !strings.Contains(first, "1 new problem(s)") || !strings.Contains(first, "1 resolved") {
		t.Errorf("first line should summarise: %q", first)
	}
}

func TestDigestOutputIsStable(t *testing.T) {
	in := []StatusSeries{
		series("http", "b", "OK", "BAD"),
		series("http", "a", "OK", "BAD"),
		series("certs", "c", "OK", "BAD"),
	}
	want := Narrate(Compare(in, 99))
	for i := 0; i < 5; i++ {
		if got := Narrate(Compare(in, 99)); got != want {
			t.Fatal("digest output is not deterministic")
		}
	}
	// Sorted by "check target", so certs comes before the two http rows.
	d := Compare(in, 99)
	if d.New[0].Check != "certs" {
		t.Errorf("changes are not sorted: %+v", d.New)
	}
}

func TestSingleRunHasNothingToCompare(t *testing.T) {
	d := Compare([]StatusSeries{series("http", "a", "BAD")}, 99)
	if !d.Empty() {
		t.Errorf("one sample is both ends of the window: %+v", d)
	}
}
