package output

import (
	"strings"
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

func TestTelegramWithProblems(t *testing.T) {
	res := engine.Result{Findings: []engine.Finding{
		{Check: "certs", Target: "api:443", Status: engine.BAD, Message: "handshake failed"},
		{Check: "http", Target: "x/health", Status: engine.WARN, Message: "slow"},
	}}
	msg, err := Telegram(res, "prod")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "checkfleet — prod") || !strings.Contains(msg, "Needs attention") {
		t.Errorf("missing header/section:\n%s", msg)
	}
	if !strings.Contains(msg, "api:443") || !strings.Contains(msg, "handshake failed") {
		t.Errorf("missing problem line:\n%s", msg)
	}
}

func TestTelegramAllGreen(t *testing.T) {
	res := engine.Result{Findings: []engine.Finding{
		{Check: "http", Target: "x", Status: engine.OK, Message: "ok"},
	}}
	msg, _ := Telegram(res, "prod")
	if !strings.Contains(msg, "All green") {
		t.Errorf("want all-green message, got:\n%s", msg)
	}
}
