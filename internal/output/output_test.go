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

func TestTextColor(t *testing.T) {
	res := engine.Result{Findings: []engine.Finding{
		{Check: "http", Target: "x/health", Status: engine.BAD, Message: "500"},
		{Check: "certs", Target: "x:443", Status: engine.OK, Message: "ok"},
	}}
	plain := Text(res)
	colored := TextColor(res)

	// Plain output must be free of ANSI escapes; colored must contain them.
	if strings.Contains(plain, "\x1b[") {
		t.Error("Text() must not emit ANSI escapes")
	}
	if !strings.Contains(colored, "\x1b[31m") || !strings.Contains(colored, ansiReset) {
		t.Errorf("TextColor() should colour BAD red and reset:\n%q", colored)
	}
	if !strings.Contains(colored, "\x1b[32m") {
		t.Error("TextColor() should colour OK green")
	}
	// Stripping the escapes must reproduce the plain rendering (alignment intact).
	stripped := colored
	for _, code := range []string{"\x1b[31m", "\x1b[32m", "\x1b[33m", "\x1b[35m", ansiReset} {
		stripped = strings.ReplaceAll(stripped, code, "")
	}
	if stripped != plain {
		t.Errorf("colored minus escapes != plain:\n%q\nvs\n%q", stripped, plain)
	}
}
