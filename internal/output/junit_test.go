package output

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

func TestJUnitValidXMLAndCounts(t *testing.T) {
	res := resultFrom([]engine.Finding{
		{Check: "http", Target: "https://x/health", Status: engine.BAD, Message: "HTTP 500"},
		{Check: "dns", Target: "x/A", Status: engine.ERROR, Message: "timeout"},
		{Check: "certs", Target: "x:443", Status: engine.WARN, Message: "near expiry"},
		{Check: "certs", Target: "y:443", Status: engine.OK, Message: "ok"},
	})
	out, err := JUnit(res, "all")
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Suite struct {
			Tests    int `xml:"tests,attr"`
			Failures int `xml:"failures,attr"`
			Errors   int `xml:"errors,attr"`
			Cases    []struct {
				Name    string `xml:"name,attr"`
				Failure *struct {
					Message string `xml:"message,attr"`
				} `xml:"failure"`
				Error *struct {
					Message string `xml:"message,attr"`
				} `xml:"error"`
			} `xml:"testsuite>testcase"`
		} `xml:"testsuite"`
	}
	// unmarshal against the <testsuites><testsuite> shape
	if err := xml.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}
	if !strings.Contains(out, "<?xml") {
		t.Error("missing XML header")
	}
}

func TestJUnitFailureAndError(t *testing.T) {
	res := resultFrom([]engine.Finding{
		{Check: "http", Target: "t", Status: engine.BAD, Message: "down"},
		{Check: "dns", Target: "d", Status: engine.ERROR, Message: "nxdomain"},
	})
	out, _ := JUnit(res, "all")
	if !strings.Contains(out, `tests="2"`) || !strings.Contains(out, `failures="1"`) || !strings.Contains(out, `errors="1"`) {
		t.Errorf("wrong counts:\n%s", out)
	}
	if !strings.Contains(out, "<failure") || !strings.Contains(out, "<error") {
		t.Errorf("missing failure/error elements:\n%s", out)
	}
}
