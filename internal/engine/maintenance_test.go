package engine

import (
	"strings"
	"testing"
	"time"
)

func TestApplyMaintenance(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	findings := []Finding{
		{Check: "certs", Target: "a:443", Status: BAD, Message: "expired"},
		{Check: "http", Target: "b/health", Status: BAD, Message: "500"},
	}

	// Active mute window for certs drops only the certs finding.
	muted := ApplyMaintenance(findings, []MaintenanceWindow{
		{Check: "certs", From: "2026-07-24T11:00:00Z", To: "2026-07-24T13:00:00Z"},
	}, now)
	if len(muted) != 1 || muted[0].Check != "http" {
		t.Fatalf("mute: want only http left, got %+v", muted)
	}

	// "warn" action caps BAD at WARN and annotates.
	warned := ApplyMaintenance(findings, []MaintenanceWindow{
		{Target: "a:443", Action: "warn"},
	}, now)
	if warned[0].Status != WARN || !strings.Contains(warned[0].Message, "[maintenance]") {
		t.Fatalf("warn: want WARN + annotation, got %+v", warned[0])
	}
	if warned[1].Status != BAD {
		t.Fatalf("warn: non-matching finding must be untouched, got %+v", warned[1])
	}

	// Inactive window (now outside range) leaves everything unchanged.
	inactive := ApplyMaintenance(findings, []MaintenanceWindow{
		{Check: "*", From: "2026-07-25T00:00:00Z"},
	}, now)
	if len(inactive) != 2 {
		t.Fatalf("inactive window must not suppress: got %+v", inactive)
	}

	// Input slice must not be mutated.
	if findings[0].Status != BAD {
		t.Fatalf("input findings mutated: %+v", findings[0])
	}
}
