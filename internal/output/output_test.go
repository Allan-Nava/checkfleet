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
			{Check: "certs", Target: "bad.example:443", Status: engine.BAD, Message: "expires in 2 days"},
			{Check: "http", Target: "https://slow.example", Status: engine.WARN, Message: "slow: 900ms"},
			{Check: "certs", Target: "ok.example:443", Status: engine.OK, Message: "expires in 200 days"},
		},
	}
}

func TestMarkdownProblemsFirst(t *testing.T) {
	md := Markdown(fixtureResult(), "all")
	problems := strings.Split(strings.Split(md, "## ⚠ Needs attention")[1], "## All results")[0]
	if !strings.Contains(problems, "bad.example") || !strings.Contains(problems, "slow.example") {
		t.Error("the problems section must contain BAD and WARN")
	}
	if strings.Contains(problems, "ok.example") {
		t.Error("the problems section must not contain OK findings")
	}
	if !strings.Contains(md, "ok.example") {
		t.Error("the full table must also contain OK findings")
	}
}

func TestTextSummary(t *testing.T) {
	text := Text(fixtureResult())
	if !strings.Contains(text, "1 OK, 1 WARN, 1 BAD, 0 ERROR") {
		t.Errorf("wrong summary:\n%s", text)
	}
}

func TestJSONHasWorst(t *testing.T) {
	s, err := JSON(fixtureResult())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, `"worst": "BAD"`) {
		t.Errorf("worst missing from JSON:\n%s", s)
	}
}
