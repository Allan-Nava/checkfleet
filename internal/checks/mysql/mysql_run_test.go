package mysql

import (
	"context"
	"strings"
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// Run against an unreachable target: the driver adapter needs a real server, so
// this is the one path of it a unit test can honestly exercise — and it asserts
// the rule the compatibility contract now guarantees: a target we could not
// measure is **ERROR**, never BAD. Conflating the two pages someone about a
// firewall rule as if the database were down (CF-158).

func TestRunUnreachableIsError(t *testing.T) {
	c := New(engine.MySQLConfig{Targets: []engine.MySQLTarget{
		{Name: "db", DSN: "monitor@tcp(127.0.0.1:1)/"},
	}})
	findings := c.Run(context.Background())
	if len(findings) == 0 {
		t.Fatal("an unreachable target must still produce a finding — silence would read as health")
	}
	if findings[0].Status != engine.ERROR {
		t.Errorf("want ERROR (could not measure), got %s: %s", findings[0].Status, findings[0].Message)
	}
	if findings[0].Target != "db" {
		t.Errorf("the finding must be keyed to the configured name, got %q", findings[0].Target)
	}
	// A DSN can carry a password; it must never reach a finding message.
	if strings.Contains(findings[0].Message, "monitor@tcp") {
		t.Errorf("the DSN leaked into the message: %q", findings[0].Message)
	}
}
