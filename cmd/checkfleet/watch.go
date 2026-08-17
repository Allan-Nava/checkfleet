// The --watch live view: re-run on an interval and redraw the terminal.

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/output"
	"github.com/Allan-Nava/checkfleet/internal/schedule"
)

// runWatch re-runs the selected checks on an interval, redrawing a live text
// view until interrupted (Ctrl-C). Maintenance and filters apply each tick.
func runWatch(jobs []engine.Job, cfg *engine.Config, filter engine.FilterOptions, interval time.Duration, color bool, limit int) error {
	// Per-module cadences apply here too (CF-178): a certs row does not need
	// re-probing every five seconds just because the frame redraws that often.
	// A module that declares none follows the interval, as before.
	sched := schedule.New(scheduleEntries(cfg, jobs), interval, limit)
	tick := sched.Tick()
	for {
		now := time.Now()
		sched.RunDue(context.Background(), now)
		res := sched.Result(cfg.Labels)
		res = engine.PostProcess(res, cfg, now)
		res.Findings = engine.Filter(res.Findings, filter)
		fmt.Print(watchFrame(res, now, tick, color))
		time.Sleep(tick)
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
