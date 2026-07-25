package output

import (
	"strings"
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

func TestCSV(t *testing.T) {
	res := engine.Result{Findings: []engine.Finding{
		{Check: "http", Target: "x/health", Status: engine.BAD, Message: "got 500, want 200"},
		{Check: "certs", Target: "a,b:443", Status: engine.OK, Message: "line1\nline2"},
	}}
	out, err := CSV(res)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if lines[0] != "status,check,target,message" {
		t.Errorf("header wrong: %q", lines[0])
	}
	if !strings.Contains(out, "BAD,http,x/health,") {
		t.Errorf("missing BAD row:\n%s", out)
	}
	// A target with a comma and a message with a newline must be quoted.
	if !strings.Contains(out, `"a,b:443"`) || !strings.Contains(out, `"line1`) {
		t.Errorf("fields with comma/newline not quoted:\n%s", out)
	}
}
