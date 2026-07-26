package main

import (
	"net"
	"testing"
)

// closedTCP returns an address that nothing is listening on (a port opened then
// immediately released), so a dial is refused fast — a deterministic non-OK.
func closedTCP(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

// The alert dedup is the heart of CF-109 — it decides when the background
// monitor is allowed to interrupt you. Pure function, exhaustive table.
func TestMonitorAlert(t *testing.T) {
	cases := []struct {
		name       string
		prev, cur  string
		wantNotify bool
		wantMsg    string
	}{
		{"first sample healthy is silent", "", "OK", false, ""},
		{"first sample broken speaks up", "", "BAD", true, "Fleet is BAD"},
		{"no change is silent", "BAD", "BAD", false, ""},
		{"worsening notifies", "WARN", "ERROR", true, "Fleet worsened: WARN → ERROR"},
		{"improving but still bad notifies", "ERROR", "WARN", true, "Fleet improved: ERROR → WARN"},
		{"recovery to OK notifies", "BAD", "OK", true, "Fleet recovered to OK"},
		{"empty current never notifies", "OK", "", false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			notify, _, msg := monitorAlert(c.prev, c.cur)
			if notify != c.wantNotify {
				t.Fatalf("notify = %v, want %v", notify, c.wantNotify)
			}
			if msg != c.wantMsg {
				t.Fatalf("msg = %q, want %q", msg, c.wantMsg)
			}
		})
	}
}

// sample() drives the real run pipeline and records the baseline. With no Wails
// context it must stay silent (no OS notification, no event) yet still track the
// worst status — that's what makes the loop testable headlessly.
func TestMonitorSampleBaseline(t *testing.T) {
	addr := startTCP(t)
	okCfg := writeConfig(t, "checkfleet.yml",
		"timeout_seconds: 5\nchecks:\n  tcp:\n    targets:\n      - address: \""+addr+"\"\n")

	app := NewApp("test") // ctx is nil → no notifications fire during the test

	worst, changed := app.sample(okCfg, "")
	if worst != "OK" {
		t.Fatalf("worst = %q, want OK for a reachable target", worst)
	}
	if changed {
		t.Fatal("a healthy first sample must not raise an alert")
	}
	if got := app.monLast; got != "OK" {
		t.Fatalf("monLast = %q, want OK", got)
	}

	// A config that can't load reports ERROR for the badge, and since the
	// baseline was OK that is a change worth alerting on.
	worst, changed = app.sample("/no/such/checkfleet.yml", "")
	if worst != "ERROR" {
		t.Fatalf("worst = %q, want ERROR for an unloadable config", worst)
	}
	if !changed {
		t.Fatal("OK → ERROR must raise an alert")
	}
}

// Muting every finding must drop the effective worst to OK, so the monitor
// neither alerts nor raises the badge (CF-111).
func TestMonitorMuteAware(t *testing.T) {
	addr := closedTCP(t)
	cfg := writeConfig(t, "checkfleet.yml",
		"timeout_seconds: 5\nchecks:\n  tcp:\n    targets:\n      - address: \""+addr+"\"\n")

	app := NewApp("test")
	rep := app.RunChecks(cfg, "")
	if rep.Worst == "OK" {
		t.Fatalf("expected a non-OK finding for a refused port, got worst=OK")
	}

	// With no mutes, the effective worst matches the raw worst.
	if got := app.effectiveWorst(cfg, rep.Findings); got != rep.Worst {
		t.Fatalf("effectiveWorst with no mutes = %q, want raw %q", got, rep.Worst)
	}

	// Mute every finding → effective worst collapses to OK.
	var keys []string
	for _, f := range rep.Findings {
		keys = append(keys, cfg+diffSep+f.Check+diffSep+f.Target)
	}
	app.SetMutedKeys(keys)
	if got := app.effectiveWorst(cfg, rep.Findings); got != "OK" {
		t.Fatalf("effectiveWorst with all muted = %q, want OK", got)
	}

	// A sample now reports OK and, coming from the (empty) baseline, no alert.
	worst, changed := app.sample(cfg, "")
	if worst != "OK" {
		t.Fatalf("sample worst with all muted = %q, want OK", worst)
	}
	if changed {
		t.Fatal("all-muted OK sample must not alert")
	}

	// Lifting the mutes brings the problem back.
	app.SetMutedKeys(nil)
	if got := app.effectiveWorst(cfg, rep.Findings); got != rep.Worst {
		t.Fatalf("effectiveWorst after unmute = %q, want %q", got, rep.Worst)
	}
}

// Start/Stop toggles the running flag and is idempotent — Stop with nothing
// running is a no-op, and the loop cancels cleanly.
func TestMonitorLifecycle(t *testing.T) {
	addr := startTCP(t)
	cfg := writeConfig(t, "checkfleet.yml",
		"timeout_seconds: 5\nchecks:\n  tcp:\n    targets:\n      - address: \""+addr+"\"\n")

	app := NewApp("test")
	if app.MonitorRunning() {
		t.Fatal("a fresh app must not be monitoring")
	}
	app.StopMonitor() // no-op, must not panic

	app.StartMonitor(cfg, "", 3600) // long interval: only the immediate sample runs
	if !app.MonitorRunning() {
		t.Fatal("StartMonitor should mark the monitor running")
	}
	app.StopMonitor()
	if app.MonitorRunning() {
		t.Fatal("StopMonitor should clear the running flag")
	}
}
