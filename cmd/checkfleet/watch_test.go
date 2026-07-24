package main

import (
	"strings"
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

func TestWatchFrame(t *testing.T) {
	res := engine.Result{Findings: []engine.Finding{
		{Check: "http", Target: "x/health", Status: engine.BAD, Message: "500"},
	}}
	frame := watchFrame(res, time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC), 5*time.Second)

	if !strings.HasPrefix(frame, "\033[H\033[2J") {
		t.Error("frame should start with the clear-screen escape")
	}
	if !strings.Contains(frame, "watch every 5s") {
		t.Errorf("frame missing header: %q", frame)
	}
	if !strings.Contains(frame, "x/health") {
		t.Error("frame should include the rendered findings")
	}
}
