package main

import (
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

func TestParseGate(t *testing.T) {
	tests := []struct {
		name      string
		exitOn    string
		exitOnBad bool
		code      int
		want      gate
		wantErr   bool
	}{
		{name: "no gate at all", code: defaultExitCode, want: gate{threshold: "", code: 2}},
		{name: "legacy --exit-on-bad", exitOnBad: true, code: defaultExitCode,
			want: gate{threshold: engine.BAD, code: 2}},
		{name: "--exit-on bad", exitOn: "bad", code: defaultExitCode,
			want: gate{threshold: engine.BAD, code: 2}},
		{name: "--exit-on warn", exitOn: "warn", code: defaultExitCode,
			want: gate{threshold: engine.WARN, code: 2}},
		{name: "--exit-on error", exitOn: "error", code: defaultExitCode,
			want: gate{threshold: engine.ERROR, code: 2}},
		{name: "case insensitive", exitOn: "WARN", code: defaultExitCode,
			want: gate{threshold: engine.WARN, code: 2}},
		{name: "custom exit code", exitOn: "bad", code: 7,
			want: gate{threshold: engine.BAD, code: 7}},
		// --exit-on is the explicit flag, so it wins over the legacy boolean
		// rather than being widened by it.
		{name: "--exit-on warn beats --exit-on-bad", exitOn: "warn", exitOnBad: true, code: defaultExitCode,
			want: gate{threshold: engine.WARN, code: 2}},

		{name: "unknown severity", exitOn: "critical", code: defaultExitCode, wantErr: true},
		{name: "ok would fail every run", exitOn: "ok", code: defaultExitCode, wantErr: true},
		{name: "exit code 0 is a no-op gate", exitOn: "bad", code: 0, wantErr: true},
		{name: "exit code 126 collides with the shell", exitOn: "bad", code: 126, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseGate(tt.exitOn, tt.exitOnBad, tt.code)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseGate(%q, %v, %d) = %+v, want error", tt.exitOn, tt.exitOnBad, tt.code, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseGate(%q, %v, %d): unexpected error: %v", tt.exitOn, tt.exitOnBad, tt.code, err)
			}
			if got != tt.want {
				t.Errorf("parseGate(%q, %v, %d) = %+v, want %+v", tt.exitOn, tt.exitOnBad, tt.code, got, tt.want)
			}
		})
	}
}

func TestGateWithImpliedThreshold(t *testing.T) {
	// --fail-on-new with no --exit-on must still be able to fail.
	g := gate{threshold: "", code: 2}.withImpliedThreshold(true)
	if g.threshold != engine.BAD {
		t.Errorf("threshold = %q, want BAD implied by --fail-on-new", g.threshold)
	}
	// An explicit threshold is never widened or narrowed.
	g = gate{threshold: engine.WARN, code: 2}.withImpliedThreshold(true)
	if g.threshold != engine.WARN {
		t.Errorf("threshold = %q, want the explicit WARN preserved", g.threshold)
	}
	// Without --fail-on-new, no gate stays no gate.
	g = gate{threshold: "", code: 2}.withImpliedThreshold(false)
	if g.threshold != "" {
		t.Errorf("threshold = %q, want no gate", g.threshold)
	}
}

func TestGateExitCode(t *testing.T) {
	all := []engine.Status{engine.OK, engine.WARN, engine.BAD, engine.ERROR}

	t.Run("no gate never trips", func(t *testing.T) {
		g := gate{threshold: "", code: 2}
		for _, worst := range all {
			if got := g.exitCode(worst); got != 0 {
				t.Errorf("worst=%s: exit %d, want 0 (a check that ran is a success)", worst, got)
			}
		}
	})

	tests := []struct {
		threshold engine.Status
		want      map[engine.Status]int // worst → exit code
	}{
		{engine.WARN, map[engine.Status]int{engine.OK: 0, engine.WARN: 2, engine.BAD: 2, engine.ERROR: 2}},
		{engine.BAD, map[engine.Status]int{engine.OK: 0, engine.WARN: 0, engine.BAD: 2, engine.ERROR: 2}},
		// Gating on error only: a BAD target is a real problem but a *measured*
		// one, so it must not trip a gate set to "the check could not run".
		{engine.ERROR, map[engine.Status]int{engine.OK: 0, engine.WARN: 0, engine.BAD: 0, engine.ERROR: 2}},
	}
	for _, tt := range tests {
		t.Run("threshold "+string(tt.threshold), func(t *testing.T) {
			g := gate{threshold: tt.threshold, code: 2}
			for _, worst := range all {
				if got := g.exitCode(worst); got != tt.want[worst] {
					t.Errorf("threshold=%s worst=%s: exit %d, want %d",
						tt.threshold, worst, got, tt.want[worst])
				}
			}
		})
	}

	t.Run("custom code is used only when it trips", func(t *testing.T) {
		g := gate{threshold: engine.BAD, code: 9}
		if got := g.exitCode(engine.WARN); got != 0 {
			t.Errorf("below threshold: exit %d, want 0", got)
		}
		if got := g.exitCode(engine.BAD); got != 9 {
			t.Errorf("at threshold: exit %d, want 9", got)
		}
	})
}
