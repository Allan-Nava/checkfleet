package output

import (
	"encoding/xml"
	"fmt"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// JUnit renders a run as a JUnit XML report: one testcase per finding, a
// <failure> for BAD, an <error> for ERROR, and a passing testcase (with a
// <system-out> note for WARN) otherwise. Suitable for a CI test tab.
func JUnit(res engine.Result, suite string) (string, error) {
	type failure struct {
		Message string `xml:"message,attr"`
		Text    string `xml:",chardata"`
	}
	type testcase struct {
		Name      string   `xml:"name,attr"`
		Classname string   `xml:"classname,attr"`
		Failure   *failure `xml:"failure,omitempty"`
		Error     *failure `xml:"error,omitempty"`
		SystemOut string   `xml:"system-out,omitempty"`
	}
	type testsuite struct {
		XMLName  xml.Name   `xml:"testsuite"`
		Name     string     `xml:"name,attr"`
		Tests    int        `xml:"tests,attr"`
		Failures int        `xml:"failures,attr"`
		Errors   int        `xml:"errors,attr"`
		Cases    []testcase `xml:"testcase"`
	}

	ts := testsuite{Name: suite, Tests: len(res.Findings)}
	for _, f := range res.Findings {
		tc := testcase{Name: f.Target, Classname: f.Check}
		switch f.Status {
		case engine.BAD:
			tc.Failure = &failure{Message: f.Message, Text: f.Message}
			ts.Failures++
		case engine.ERROR:
			tc.Error = &failure{Message: f.Message, Text: f.Message}
			ts.Errors++
		case engine.WARN:
			tc.SystemOut = "WARN: " + f.Message
		}
		ts.Cases = append(ts.Cases, tc)
	}
	out, err := xml.MarshalIndent(struct {
		XMLName xml.Name `xml:"testsuites"`
		Suite   testsuite
	}{Suite: ts}, "", "  ")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%s\n", xml.Header, out), nil
}
