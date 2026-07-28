package postgres

import (
	"context"
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// Run against an unreachable target: the driver adapter needs a real server, so
// this is the one path of it a unit test can honestly exercise — and it asserts
// the rule the compatibility contract now guarantees: a target we could not
// measure is **ERROR**, never BAD. Conflating the two pages someone about a
// firewall rule as if the database were down (CF-158).

func TestRunUnreachableIsError(t *testing.T) {
	c := New(engine.PostgresConfig{Targets: []engine.PostgresTarget{
		{Name: "pg", DSN: "host=127.0.0.1 port=1 user=monitor dbname=postgres connect_timeout=1"},
	}})
	findings := c.Run(context.Background())
	if len(findings) == 0 {
		t.Fatal("an unreachable target must still produce a finding")
	}
	if findings[0].Status != engine.ERROR {
		t.Errorf("want ERROR (could not measure), got %s: %s", findings[0].Status, findings[0].Message)
	}
}
