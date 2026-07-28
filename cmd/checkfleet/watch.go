// The --watch live view: re-run on an interval and redraw the terminal.

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/output"
)

// runWatch re-runs the selected checks on an interval, redrawing a live text
// view until interrupted (Ctrl-C). Maintenance and filters apply each tick.
func runWatch(jobs []engine.Job, cfg *engine.Config, filter engine.FilterOptions, interval time.Duration, color bool, limit int) error {
	for {
		res := engine.RunJobsLimited(context.Background(), jobs, limit)
		res.Labels = cfg.Labels
		res.Findings = engine.ApplyMaintenance(res.Findings, cfg.Maintenance, time.Now())
		res.Findings = engine.Filter(res.Findings, filter)
		fmt.Print(watchFrame(res, time.Now(), interval, color))
		time.Sleep(interval)
	}
}

// watchFrame renders one live frame: clear the screen, a header, then the text
// output. Kept separate so it can be tested without the loop.
func watchFrame(res engine.Result, now time.Time, interval time.Duration, color bool) string {
	body := output.Text(res)
	if color {
		body = output.TextColor(res)
	}
	return fmt.Sprintf("\033[H\033[2Jcheckfleet — watch every %s — %s\n\n%s",
		interval, now.Format("15:04:05"), body)
}
