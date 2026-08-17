package alert

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

var t0 = time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)

func TestFirstNotificationAlwaysGoes(t *testing.T) {
	send, why := Decide(Notified{}, engine.BAD, t0, Policy{})
	if !send {
		t.Fatal("the first notification must always be sent")
	}
	if !strings.Contains(why, "first") {
		t.Errorf("reason = %q", why)
	}
}

// TestWithoutAPolicyItStaysQuiet — the historical behaviour: notify once, then
// nothing until it recovers. Kept as the default so nobody who has not asked
// for this sees a change.
func TestWithoutAPolicyItStaysQuiet(t *testing.T) {
	prev := Notified{At: t0, Status: engine.BAD}
	send, why := Decide(prev, engine.BAD, t0.Add(72*time.Hour), Policy{})
	if send {
		t.Error("with no renotify_after, a still-open problem must not re-fire")
	}
	if !strings.Contains(why, "renotify_after") {
		t.Errorf("the reason should name the setting that would change it: %q", why)
	}
}

func TestReNotifiesAfterTheInterval(t *testing.T) {
	prev := Notified{At: t0, Status: engine.BAD}
	p := Policy{After: 4 * time.Hour}

	if send, _ := Decide(prev, engine.BAD, t0.Add(3*time.Hour), p); send {
		t.Error("three hours into a four-hour interval must stay quiet")
	}
	send, why := Decide(prev, engine.BAD, t0.Add(4*time.Hour), p)
	if !send {
		t.Error("at the interval it should fire")
	}
	if !strings.Contains(why, "still failing") {
		t.Errorf("reason = %q", why)
	}
}

// TestWorseningFiresImmediately — a situation that deteriorated is new
// information, and holding it for the timer is the wrong trade.
func TestWorseningFiresImmediately(t *testing.T) {
	prev := Notified{At: t0, Status: engine.WARN}
	p := Policy{After: 24 * time.Hour, OnWorsening: true}

	send, why := Decide(prev, engine.BAD, t0.Add(time.Minute), p)
	if !send {
		t.Fatal("WARN → BAD should fire even one minute in")
	}
	if !strings.Contains(why, "worsened") {
		t.Errorf("reason = %q", why)
	}
	// Improving does not, and neither does standing still.
	if send, _ := Decide(Notified{At: t0, Status: engine.BAD}, engine.WARN, t0.Add(time.Minute), p); send {
		t.Error("BAD → WARN is not a worsening")
	}
	if send, _ := Decide(prev, engine.WARN, t0.Add(time.Minute), p); send {
		t.Error("an unchanged status is not a worsening")
	}
}

func TestWorseningIsOptOut(t *testing.T) {
	prev := Notified{At: t0, Status: engine.WARN}
	if send, _ := Decide(prev, engine.BAD, t0.Add(time.Minute), Policy{After: time.Hour}); send {
		t.Error("without OnWorsening the timer alone decides")
	}
}

// TestASequenceOverThreeDays walks the case the item describes, so the policy
// is judged on the behaviour an operator actually lives with.
func TestASequenceOverThreeDays(t *testing.T) {
	p := Policy{After: 24 * time.Hour, OnWorsening: true}
	var state Notified
	var fired []string

	for h := 0; h <= 72; h++ {
		at := t0.Add(time.Duration(h) * time.Hour)
		status := engine.BAD
		if h >= 30 && h < 40 {
			status = engine.ERROR // it gets worse for a while
		}
		if send, why := Decide(state, status, at, p); send {
			fired = append(fired, why)
			state = Notified{At: at, Status: status}
		}
	}
	// Expected: the first, one at +24h, the worsening at +30h, then daily.
	if len(fired) < 4 || len(fired) > 6 {
		t.Errorf("fired %d times over three days, want a handful: %v", len(fired), fired)
	}
	if !strings.Contains(fired[0], "first") {
		t.Errorf("the first event should be the first notification: %q", fired[0])
	}
	var worsened bool
	for _, f := range fired {
		if strings.Contains(f, "worsened") {
			worsened = true
		}
	}
	if !worsened {
		t.Error("the deterioration should have fired on its own")
	}
}

func TestStateRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alert-state.json")
	s, err := LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Sent) != 0 {
		t.Error("a missing state file is an empty state, not an error")
	}
	s.Record("postgres/db-01", t0, engine.BAD)
	if err := SaveState(path, s); err != nil {
		t.Fatal(err)
	}
	back, err := LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	got := back.Sent["postgres/db-01"]
	if !got.At.Equal(t0) || got.Status != engine.BAD {
		t.Errorf("round trip lost data: %+v", got)
	}
}

// TestStateIsWrittenTightly — it records what is failing and when, which is
// operational detail about the fleet.
func TestStateIsWrittenTightly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.json")
	s, _ := LoadState(path)
	s.Record("a/b", t0, engine.BAD)
	if err := SaveState(path, s); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm()&0o077 != 0 {
		t.Errorf("state file mode is %04o, want no group/other access", fi.Mode().Perm())
	}
}

// TestForgetMakesTheNextOneFirstAgain: after a recovery the timer must not
// carry over, or a problem returning a month later would be judged against an
// ancient notification.
func TestForgetMakesTheNextOneFirstAgain(t *testing.T) {
	s, _ := LoadState("")
	s.Record("a/b", t0, engine.BAD)
	s.Forget("a/b")
	send, why := Decide(s.Sent["a/b"], engine.BAD, t0.Add(time.Hour), Policy{After: 24 * time.Hour})
	if !send || !strings.Contains(why, "first") {
		t.Errorf("after a recovery the next problem is a first notification: %v / %q", send, why)
	}
}

func TestCorruptStateIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadState(path); err == nil {
		t.Error("a corrupt state file must be reported, not silently treated as empty")
	}
}
