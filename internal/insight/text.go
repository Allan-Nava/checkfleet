package insight

import (
	"fmt"
	"strings"
	"time"
)

// TextOptions carries the values the renderer needs to caption a section but
// cannot recover from the report itself (the threshold a forecast aimed at, the
// objective a budget was measured against).
type TextOptions struct {
	Threshold float64
	Objective float64
	Z         float64
	// Indent prefixes every line, for embedding under a run's output.
	Indent string
}

// Text renders a Report for a terminal. Shared by `checkfleet insight` and by
// the `check --history` text output, so an insight reads identically wherever
// it appears (CF-173).
func Text(r Report, o TextOptions) string {
	var b strings.Builder
	if r.Digest != nil {
		b.WriteString(Narrate(*r.Digest))
	}
	if r.Score != nil {
		section(&b, r.Digest != nil)
		fmt.Fprintf(&b, "Fleet health: %.1f/100 over %d finding(s)", r.Score.Value, r.Score.Findings)
		if r.Score.Unstable > 0 {
			fmt.Fprintf(&b, ", %d unstable target(s)", r.Score.Unstable)
		}
		b.WriteString("\n")
		for _, name := range r.Score.Worst {
			fmt.Fprintf(&b, "  %-14s %5.1f\n", name, r.Score.Modules[name])
		}
	}
	if len(r.Clusters) > 0 {
		section(&b, b.Len() > 0)
		b.WriteString("Correlated failures:\n")
		for _, c := range r.Clusters {
			fmt.Fprintf(&b, "  %d failures share the same %s: %s\n", c.Size, c.Dimension, c.Value)
		}
	}
	if len(r.Recovery) > 0 {
		section(&b, b.Len() > 0)
		fmt.Fprintf(&b, "Recovery, over %d run(s):\n", r.Runs)
		for _, rr := range r.Recovery {
			fmt.Fprintf(&b, "  %-10s %-38s", rr.Check, rr.Target)
			if rr.Down {
				since := (time.Duration(rr.OngoingSec) * time.Second).Round(time.Minute)
				switch {
				case since == 0:
					// Sub-minute, or every sample inside the same instant. A
					// duration of "0s" reads as a measurement; "down" is the fact.
					b.WriteString("  down")
					if rr.Unresolved {
						b.WriteString(" (started before the window)")
					}
				case rr.Unresolved:
					fmt.Fprintf(&b, "  down for at least %s (started before the window)", since)
				default:
					fmt.Fprintf(&b, "  down for %s", since)
				}
				if rr.Outages > 0 {
					fmt.Fprintf(&b, ", usually back in ~%s", dur(rr.MeanSec))
				}
				b.WriteString("\n")
				continue
			}
			fmt.Fprintf(&b, "  up · %d outage(s), MTTR ~%s (p50 %s, p90 %s)\n",
				rr.Outages, dur(rr.MeanSec), dur(rr.P50Sec), dur(rr.P90Sec))
		}
	}
	if len(r.Anomalies) > 0 {
		section(&b, b.Len() > 0)
		fmt.Fprintf(&b, "Deviation from each metric's own baseline (z >= %g), over %d run(s):\n", orFloat(o.Z, 3), r.Runs)
		for _, a := range r.Anomalies {
			fmt.Fprintf(&b, "  %-10s %-38s %8.2f%s", a.Check, a.Target, a.Latest, a.Unit)
			switch {
			case a.Note != "":
				fmt.Fprintf(&b, "  %s\n", a.Note)
			case a.Deviating && a.Ratio > 0:
				fmt.Fprintf(&b, "  %.1fx its norm of %.2f%s (z=%+.1f)\n", a.Ratio, a.Baseline, a.Unit, a.Z)
			case a.Deviating:
				fmt.Fprintf(&b, "  off its norm of %.2f%s (z=%+.1f)\n", a.Baseline, a.Unit, a.Z)
			default:
				fmt.Fprintf(&b, "  normal (baseline %.2f%s)\n", a.Baseline, a.Unit)
			}
		}
	}
	if len(r.Forecasts) > 0 {
		section(&b, b.Len() > 0)
		fmt.Fprintf(&b, "Forecast to %g, over %d run(s):\n", o.Threshold, r.Runs)
		for _, f := range r.Forecasts {
			fmt.Fprintf(&b, "  %-10s %-38s %8.2f%s", f.Check, f.Target, f.Latest, f.Unit)
			if f.ETA != "" {
				fmt.Fprintf(&b, "  crosses in ~%.1f days (%+.2f%s/day, R²=%.2f)\n", f.InDays, f.Slope, f.Unit, f.R2)
				continue
			}
			fmt.Fprintf(&b, "  %s\n", f.Note)
		}
	}
	if len(r.Budgets) > 0 {
		section(&b, b.Len() > 0)
		fmt.Fprintf(&b, "Error budget against %.4g%% availability, over %d run(s):\n", o.Objective*100, r.Runs)
		for _, bg := range r.Budgets {
			fmt.Fprintf(&b, "  %-10s %-38s %6.2f%% up", bg.Check, bg.Target, bg.Availability*100)
			switch {
			case bg.Note != "":
				fmt.Fprintf(&b, "  %s\n", bg.Note)
			case bg.Exhausted != "":
				fmt.Fprintf(&b, "  %.0f%% of budget left, fast burn %.1fx → gone %s\n", bg.Remaining*100, bg.FastBurn, bg.Exhausted)
			default:
				fmt.Fprintf(&b, "  %.0f%% of budget left (burn %.2fx)\n", bg.Remaining*100, bg.SlowBurn)
			}
		}
	}
	for _, n := range r.Notes {
		section(&b, b.Len() > 0)
		fmt.Fprintf(&b, "%s\n", n)
	}
	if o.Indent == "" {
		return b.String()
	}
	return indent(b.String(), o.Indent)
}

// section writes the blank line between blocks, but only between them.
func section(b *strings.Builder, needed bool) {
	if needed {
		b.WriteString("\n")
	}
}

func indent(s, prefix string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = prefix + l
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func dur(seconds int64) string {
	return (time.Duration(seconds) * time.Second).Round(time.Minute).String()
}

func orFloat(v, def float64) float64 {
	if v == 0 {
		return def
	}
	return v
}
