package output

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

func resultFrom(findings []engine.Finding) engine.Result {
	return engine.Result{Findings: findings, Started: time.Unix(0, 0), Duration: time.Millisecond}
}

func TestSlackValidJSONAndBlocks(t *testing.T) {
	res := resultFrom([]engine.Finding{
		{Check: "http", Target: "https://x/health", Status: engine.BAD, Message: "HTTP 500"},
		{Check: "certs", Target: "x:443", Status: engine.OK, Message: "ok"},
	})
	out, err := Slack(res, "all")
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Blocks []struct {
			Type string `json:"type"`
			Text *struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"text"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("JSON non valido: %v", err)
	}
	if payload.Blocks[0].Type != "header" || !strings.Contains(payload.Blocks[0].Text.Text, "all") {
		t.Errorf("primo blocco: atteso header con titolo, avuto %+v", payload.Blocks[0])
	}
	// header + summary + 1 problema (l'OK non compare)
	if len(payload.Blocks) != 3 {
		t.Errorf("attesi 3 blocchi (header, summary, 1 problema), avuti %d", len(payload.Blocks))
	}
	if !strings.Contains(out, "HTTP 500") {
		t.Errorf("il problema BAD dovrebbe comparire nel payload")
	}
}

func TestSlackAllGreen(t *testing.T) {
	res := resultFrom([]engine.Finding{{Check: "certs", Target: "x", Status: engine.OK, Message: "ok"}})
	out, err := Slack(res, "certs")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "white_check_mark") {
		t.Errorf("tutto verde: atteso messaggio positivo, avuto %s", out)
	}
}

func TestSlackCapsProblems(t *testing.T) {
	var findings []engine.Finding
	for i := 0; i < maxSlackProblems+5; i++ {
		findings = append(findings, engine.Finding{Check: "http", Target: "t", Status: engine.BAD, Message: "down"})
	}
	out, err := Slack(resultFrom(findings), "all")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "e altri 5 problemi") {
		t.Errorf("cap problemi: attesa nota di troncamento, avuto %s", out)
	}
}
