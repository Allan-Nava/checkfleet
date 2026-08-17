package alert

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// Policy decides whether an alert that is still failing should be sent again
// (CF-176).
//
// `alert` deduplicates by check+target, which is right for the first
// notification and useless afterwards: a BAD that lasts three days either
// re-fires on every run — so people mute the channel — or never fires again and
// is forgotten. Neither is a decision anyone made; both are what you get when
// there is no way to say.
type Policy struct {
	// After is how long to wait before re-notifying about a problem that is
	// still open. Zero means never: notify once and stay quiet until it
	// recovers.
	After time.Duration
	// OnWorsening re-notifies immediately when the status gets worse
	// (WARN→BAD, BAD→ERROR) regardless of After. A situation that deteriorated
	// is new information, and holding it for the timer is the wrong trade.
	OnWorsening bool
}

// Notified records what was last sent for one alert key.
type Notified struct {
	At     time.Time     `json:"at"`
	Status engine.Status `json:"status"`
}

// Decide reports whether to send, and why — the reason is returned so the
// dry-run output and the logs can say what the policy did rather than leaving
// the operator to infer it from silence.
//
// prev is the zero value when nothing was ever sent for this key.
func Decide(prev Notified, curr engine.Status, at time.Time, p Policy) (bool, string) {
	if prev.At.IsZero() {
		return true, "first notification"
	}
	if p.OnWorsening && engine.AtLeast(curr, prev.Status) && curr != prev.Status {
		return true, fmt.Sprintf("worsened %s → %s", prev.Status, curr)
	}
	if p.After <= 0 {
		return false, "already notified; renotify_after is not set"
	}
	if elapsed := at.Sub(prev.At); elapsed >= p.After {
		return true, fmt.Sprintf("still failing after %s", elapsed.Round(time.Minute))
	}
	return false, fmt.Sprintf("notified %s ago, waiting for %s",
		at.Sub(prev.At).Round(time.Minute), p.After)
}

// State is the persisted notification memory, keyed by check/target.
//
// It is a separate file from --history on purpose: the history is a contractual
// format (compatibility §7) that other things read, and notification bookkeeping
// is neither interesting to them nor stable enough to freeze.
type State struct {
	Sent map[string]Notified `json:"sent"`
}

// LoadState reads the state file. A missing file is an empty state, not an
// error: the first run has nothing to remember.
func LoadState(path string) (State, error) {
	s := State{Sent: map[string]Notified{}}
	if path == "" {
		return s, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return s, err
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return State{Sent: map[string]Notified{}}, fmt.Errorf("parse %s: %w", path, err)
	}
	if s.Sent == nil {
		s.Sent = map[string]Notified{}
	}
	return s, nil
}

// SaveState writes the state atomically — a half-written file would make the
// next run think nothing was ever sent and re-page the whole fleet.
func SaveState(path string, s State) error {
	if path == "" {
		return nil
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".checkfleet-alert-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, 0o600); err != nil {
		return err
	}
	return os.Rename(name, path)
}

// Forget drops the record for a key, called when the problem resolves so the
// next occurrence is a first notification again rather than an old timer.
func (s State) Forget(key string) { delete(s.Sent, key) }

// Record stores what was just sent.
func (s State) Record(key string, at time.Time, status engine.Status) {
	s.Sent[key] = Notified{At: at, Status: status}
}
