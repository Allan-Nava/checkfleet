package main

import (
	"context"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/gen2brain/beeep"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Background monitor (CF-109). A Go-driven ticker keeps running the fleet even
// while you're on another view, and — crucially — a native notification fires
// *only when the worst status changes*, not on every sample, so a persistently
// bad fleet doesn't nag you. Each sample is emitted to the frontend as a
// "monitor:sample" event carrying the full report, so the UI updates live and
// off-thread.
//
// This is the verifiable, zero-dep slice of CF-109. A colored menu-bar/tray icon
// needs a systray, which Wails v2 doesn't provide (it lands in Wails v3); that
// piece is deferred rather than pulled in as a fragile main-thread dependency.

// monitorMinInterval floors the poll interval — a monitor is a background watch,
// not a load generator, and a runaway 1s loop would hammer the targets.
const monitorMinInterval = 5

// MonitorSample is emitted to the frontend after every background run.
type MonitorSample struct {
	Report  Report `json:"report"`
	Worst   string `json:"worst"`
	Changed bool   `json:"changed"` // did the worst status change vs the last sample?
	Running bool   `json:"running"`
}

// monitorAlert decides whether a worst-status change deserves a native
// notification, and what it should say. It is pure (no side effects) so the
// dedup logic is unit-testable. prev == "" means "no prior sample": starting the
// monitor on a healthy fleet is silent, but starting on an already-broken one
// tells you straight away.
func monitorAlert(prev, cur string) (notify bool, title, msg string) {
	if cur == "" || prev == cur {
		return false, "", ""
	}
	switch {
	case prev == "":
		if cur == "OK" {
			return false, "", ""
		}
		return true, "checkfleet — monitoring", "Fleet is " + cur
	case statusRank[cur] > statusRank[prev]:
		return true, "checkfleet — degraded", "Fleet worsened: " + prev + " → " + cur
	case cur == "OK":
		return true, "checkfleet — recovered", "Fleet recovered to OK"
	default:
		return true, "checkfleet — improved", "Fleet improved: " + prev + " → " + cur
	}
}

// SetMutedKeys replaces the set of muted finding keys (CF-111). The frontend
// owns the mute store (localStorage) and pushes the currently-active keys here
// whenever they change, so the Go monitor can honour them. Key format is
// configPath \x1f check \x1f target — the same key the JS side builds.
func (a *App) SetMutedKeys(keys []string) {
	set := make(map[string]bool, len(keys))
	for _, k := range keys {
		set[k] = true
	}
	a.mutedMu.Lock()
	a.mutedKeys = set
	a.mutedMu.Unlock()
}

// effectiveWorst is the worst status over the findings that are NOT muted — the
// status the monitor actually alerts on. With no mutes it equals the raw worst.
func (a *App) effectiveWorst(configPath string, findings []engine.Finding) string {
	a.mutedMu.Lock()
	muted := a.mutedKeys
	a.mutedMu.Unlock()

	worst := "OK"
	for _, f := range findings {
		if len(muted) > 0 && muted[configPath+diffSep+f.Check+diffSep+f.Target] {
			continue
		}
		if statusRank[string(f.Status)] > statusRank[worst] {
			worst = string(f.Status)
		}
	}
	return worst
}

// StartMonitor begins polling configPath every everySeconds (floored at
// monitorMinInterval), running an immediate first sample. Any prior monitor is
// stopped first, so calling this on a config/interval change just re-points it.
func (a *App) StartMonitor(configPath, stack string, everySeconds int) {
	if everySeconds < monitorMinInterval {
		everySeconds = monitorMinInterval
	}
	a.StopMonitor()

	ctx, cancel := context.WithCancel(context.Background())
	a.monMu.Lock()
	a.monCancel = cancel
	a.monLast = ""
	a.monMu.Unlock()

	go a.monitorLoop(ctx, configPath, stack, time.Duration(everySeconds)*time.Second)
}

// StopMonitor cancels the background loop (a no-op if none is running) and lets
// the frontend clear its indicator.
func (a *App) StopMonitor() {
	a.monMu.Lock()
	cancel := a.monCancel
	a.monCancel = nil
	a.monMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if a.ctx != nil {
		wruntime.EventsEmit(a.ctx, "monitor:stopped")
	}
}

// MonitorRunning reports whether a background monitor is active (for the UI to
// restore its toggle after a reload).
func (a *App) MonitorRunning() bool {
	a.monMu.Lock()
	defer a.monMu.Unlock()
	return a.monCancel != nil
}

func (a *App) monitorLoop(ctx context.Context, configPath, stack string, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	a.sample(configPath, stack) // sample once right away, then on the tick
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.sample(configPath, stack)
		}
	}
}

// sample runs one background pass, records the new worst status, fires a native
// notification if (and only if) it changed, and emits the result to the
// frontend. It returns the worst status and whether it changed, so the loop is
// testable without the Wails runtime (the notification and event emit are
// skipped when there is no GUI context, keeping tests silent).
func (a *App) sample(configPath, stack string) (string, bool) {
	rep := a.RunChecks(configPath, stack)
	// Alert on the mute-aware worst: a snoozed finding must not re-notify (CF-111).
	worst := a.effectiveWorst(configPath, rep.Findings)
	if rep.Err != "" {
		worst = "ERROR" // a config that won't load is an ERROR for the badge
	}

	a.monMu.Lock()
	prev := a.monLast
	a.monLast = worst
	a.monMu.Unlock()

	notify, title, msg := monitorAlert(prev, worst)
	if notify && a.ctx != nil {
		_ = beeep.Notify(title, msg, "")
	}
	if a.ctx != nil {
		wruntime.EventsEmit(a.ctx, "monitor:sample", MonitorSample{
			Report: rep, Worst: worst, Changed: notify, Running: true,
		})
	}
	return worst, notify
}
