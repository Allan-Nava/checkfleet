package output

import (
	"strings"
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

func fixtureResult() engine.Result {
	return engine.Result{
		Started:  time.Date(2026, 7, 23, 10, 0, 0, 0, time.UTC),
		Duration: 1500 * time.Millisecond,
		Findings: []engine.Finding{
			{Check: "certs", Target: "bad.example:443", Status: engine.BAD, Message: "scade tra 2 giorni"},
			{Check: "http", Target: "https://slow.example", Status: engine.WARN, Message: "lento: 900ms"},
			{Check: "certs", Target: "ok.example:443", Status: engine.OK, Message: "scade tra 200 giorni"},
		},
	}
}

func TestMarkdownProblemsFirst(t *testing.T) {
	md := Markdown(fixtureResult(), "all")
	problems := strings.Split(strings.Split(md, "## ⚠ Da guardare")[1], "## Tutti i risultati")[0]
	if !strings.Contains(problems, "bad.example") || !strings.Contains(problems, "slow.example") {
		t.Error("la sezione problemi deve contenere BAD e WARN")
	}
	if strings.Contains(problems, "ok.example") {
		t.Error("la sezione problemi non deve contenere gli OK")
	}
	if !strings.Contains(md, "ok.example") {
		t.Error("la tabella completa deve contenere anche gli OK")
	}
}

func TestTextSummary(t *testing.T) {
	text := Text(fixtureResult())
	if !strings.Contains(text, "1 OK, 1 WARN, 1 BAD, 0 ERROR") {
		t.Errorf("summary sbagliata:\n%s", text)
	}
}

func TestJSONHasWorst(t *testing.T) {
	s, err := JSON(fixtureResult())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, `"worst": "BAD"`) {
		t.Errorf("worst mancante nel JSON:\n%s", s)
	}
}
