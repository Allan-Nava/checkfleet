package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/coverage"
	"github.com/Allan-Nava/checkfleet/internal/doctor"
	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/output"
)

// runDoctor is the preflight: why isn't this working? It reports on the
// environment (variables, secret files, target syntax, reachability) rather
// than on the services.
//
// It exits 0 whatever it finds — the M31 rule for diagnostics. The one thing it
// must never do is refuse to run: a broken config is exactly when you need it,
// so the variable scan happens on the raw file before any parsing, and a config
// that fails to load is itself reported as a finding.
//
//	checkfleet doctor --config checkfleet.yml [--output text|json] [--no-probe]
func runDoctor(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	configPath := fs.String("config", "checkfleet.yml", "YAML config file")
	stack := fs.String("stack", "", "comma-separated stack profiles overlaid in order (last wins)")
	format := fs.String("output", "text", "output format: text or json")
	noProbe := fs.Bool("no-probe", false, "skip the network probes (config and variables only)")
	timeout := fs.Duration("probe-timeout", 3*time.Second, "per-probe timeout for DNS and TCP")
	maxConc := fs.Int("max-concurrency", 16, "cap on probes running at once")
	noColor := fs.Bool("no-color", false, "disable ANSI colour (also honours NO_COLOR)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *format != "text" && *format != "json" {
		return fmt.Errorf("unknown --output %q (use text or json)", *format)
	}

	started := time.Now()
	var findings []engine.Finding

	// 1. Variables, from the raw text. Done first and independently of loading:
	//    an unset ${NAME} expands to "" silently, and naming it is the whole
	//    point of this command.
	refs, err := engine.ScanVars(*configPath)
	if err != nil {
		// The config file itself is unreadable — systemic, nothing else to do.
		return err
	}
	findings = append(findings, doctor.Env(refs)...)

	// 2. The config, whose failure to load is a finding rather than an abort.
	cfg, cfgErr := loadConfig(*configPath, *stack)
	if cfgErr != nil {
		findings = append(findings, engine.Finding{
			Check: "config", Target: *configPath, Status: engine.BAD,
			Message: cfgErr.Error(),
		})
	} else {
		findings = append(findings, doctor.Config(cfg, *configPath)...)

		// 3. Target syntax, then 4. reachability.
		targets := coverage.Targets(cfg)
		findings = append(findings, doctor.Targets(targets)...)
		if !*noProbe {
			findings = append(findings, doctor.Probe(context.Background(), targets, *timeout, *maxConc)...)
		}
	}

	res := engine.Result{
		Findings: engine.SortFindings(findings),
		Started:  started,
		Duration: time.Since(started),
	}

	if *format == "json" {
		s, err := output.JSON(res)
		if err != nil {
			return err
		}
		fmt.Println(s)
		return nil
	}

	color := !*noColor && os.Getenv("NO_COLOR") == "" && isTerminal(os.Stdout)
	if color {
		fmt.Print(output.TextColor(res))
	} else {
		fmt.Print(output.Text(res))
	}
	return nil
}
