package mongodb

import (
	"context"
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// Run against an unreachable target: the driver adapter needs a real server, so
// this is the one path of it a unit test can honestly exercise — and it asserts
// the rule the compatibility contract now guarantees: a target we could not
// measure is **ERROR**, never BAD. Conflating the two pages someone about a
// firewall rule as if the database were down (CF-158).

func TestRunUnreachableIsError(t *testing.T) {
	c := New(engine.MongoDBConfig{Targets: []engine.MongoDBTarget{
		{Name: "mongo", URI: "mongodb://127.0.0.1:1"},
	}})
	// The driver's own server-selection timeout is 30s; the engine always passes
	// a bounded context, and so must this test — a unit suite that takes half a
	// minute to learn a port is closed stops being run.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	findings := c.Run(ctx)
	if len(findings) == 0 {
		t.Fatal("an unreachable target must still produce a finding")
	}
	if findings[0].Status != engine.ERROR {
		t.Errorf("want ERROR (could not measure), got %s: %s", findings[0].Status, findings[0].Message)
	}
}
