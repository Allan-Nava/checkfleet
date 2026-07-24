package ntp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

func check(res result, err error) *Check {
	c := New(engine.NTPConfig{Targets: []string{"ntp.example"}, OffsetWarnMS: 100, OffsetCritMS: 1000})
	c.query = func(context.Context, string) (result, error) { return res, err }
	return c
}

func run(t *testing.T, c *Check) engine.Finding {
	t.Helper()
	f := c.Run(context.Background())
	if len(f) != 1 {
		t.Fatalf("want 1 finding, got %d", len(f))
	}
	return f[0]
}

func TestOffsetThresholds(t *testing.T) {
	cases := []struct {
		off  time.Duration
		want engine.Status
	}{
		{20 * time.Millisecond, engine.OK},
		{-20 * time.Millisecond, engine.OK},
		{250 * time.Millisecond, engine.WARN},
		{-2 * time.Second, engine.BAD},
	}
	for _, tc := range cases {
		if got := run(t, check(result{Offset: tc.off, Stratum: 3}, nil)); got.Status != tc.want {
			t.Errorf("offset %s: want %s, got %s (%s)", tc.off, tc.want, got.Status, got.Message)
		}
	}
}

func TestUnsyncedStratumIsBad(t *testing.T) {
	if got := run(t, check(result{Offset: 0, Stratum: 16}, nil)); got.Status != engine.BAD {
		t.Errorf("stratum 16: want BAD, got %s (%s)", got.Status, got.Message)
	}
	if got := run(t, check(result{Offset: 0, Stratum: 0}, nil)); got.Status != engine.BAD {
		t.Errorf("stratum 0 (KoD): want BAD, got %s (%s)", got.Status, got.Message)
	}
}

func TestQueryErrorIsError(t *testing.T) {
	if got := run(t, check(result{}, errors.New("timeout"))); got.Status != engine.ERROR {
		t.Errorf("query failed: want ERROR, got %s (%s)", got.Status, got.Message)
	}
}

func TestNTPTimestampConversion(t *testing.T) {
	// Encode a known Unix time as an NTP timestamp and round-trip it.
	want := time.Unix(1_700_000_000, 0).UTC()
	var ts uint64 = uint64(want.Unix()+ntpEpochOffset) << 32
	got := ntpToTime(ts).UTC()
	if got.Unix() != want.Unix() {
		t.Errorf("NTP conversion: want %v, got %v", want, got)
	}
}
