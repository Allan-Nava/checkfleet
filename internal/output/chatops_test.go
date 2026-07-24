package output

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

func TestDiscordAndTeamsValid(t *testing.T) {
	renderers := map[string]func(engine.Result, string) (string, error){
		"discord": Discord,
		"teams":   Teams,
	}
	for name, fn := range renderers {
		out, err := fn(fixtureResult(), "all")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var v any
		if err := json.Unmarshal([]byte(out), &v); err != nil {
			t.Errorf("%s: invalid JSON: %v\n%s", name, err, out)
		}
		if !strings.Contains(out, "checkfleet") {
			t.Errorf("%s: missing title", name)
		}
		if !strings.Contains(out, "bad.example") {
			t.Errorf("%s: BAD finding not included\n%s", name, out)
		}
	}
}

func TestDiscordCapsProblems(t *testing.T) {
	var findings []engine.Finding
	for i := 0; i < maxChatProblems+5; i++ {
		findings = append(findings, engine.Finding{Check: "http", Target: "t", Status: engine.BAD, Message: "down"})
	}
	out, err := Discord(engine.Result{Findings: findings}, "all")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "5 more") {
		t.Errorf("problem cap: want a truncation note, got:\n%s", out)
	}
}

func TestChatOpsAllGreen(t *testing.T) {
	res := engine.Result{Findings: []engine.Finding{{Check: "certs", Target: "x", Status: engine.OK, Message: "ok"}}}
	for _, fn := range []func(engine.Result, string) (string, error){Discord, Teams} {
		out, err := fn(res, "certs")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(strings.ToLower(out), "all green") {
			t.Errorf("all-green message missing:\n%s", out)
		}
	}
}
