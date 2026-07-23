// checkfleet: a fleet of infrastructure checks with one binary.
//
//	checkfleet check all   --config checkfleet.yml [--output text|markdown|json]
//	checkfleet check certs --config checkfleet.yml
//	checkfleet version
//
// Exit code: 0 also on WARN/BAD findings (a check that ran IS a success —
// gate on the output instead, or pass --exit-on-bad to get exit 2 when any
// BAD/ERROR finding is present). Non-zero otherwise only for systemic errors
// (unreadable config, unknown module).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/checks/certs"
	"github.com/Allan-Nava/checkfleet/internal/checks/haproxy"
	"github.com/Allan-Nava/checkfleet/internal/checks/httpcheck"
	"github.com/Allan-Nava/checkfleet/internal/checks/nats"
	"github.com/Allan-Nava/checkfleet/internal/checks/stream"
	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/output"
)

var version = "dev" // injected at build time via -ldflags

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(64)
	}
	switch os.Args[1] {
	case "version":
		fmt.Println("checkfleet", version)
	case "check":
		if err := runCheck(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "checkfleet:", err)
			os.Exit(1)
		}
	default:
		usage()
		os.Exit(64)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `uso:
  checkfleet check <all|certs|http|nats|haproxy|stream> --config checkfleet.yml [--output text|markdown|json] [--exit-on-bad]
  checkfleet version`)
}

func runCheck(args []string) error {
	if len(args) < 1 {
		usage()
		return fmt.Errorf("modulo mancante")
	}
	module := args[0]

	fs := flag.NewFlagSet("check", flag.ExitOnError)
	configPath := fs.String("config", "checkfleet.yml", "file di configurazione YAML")
	format := fs.String("output", "text", "formato: text, markdown, json")
	exitOnBad := fs.Bool("exit-on-bad", false, "exit code 2 se presenti finding BAD/ERROR")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	cfg, err := engine.LoadConfig(*configPath)
	if err != nil {
		return err
	}

	var selected []engine.Check
	add := func(name string, build func() engine.Check, configured bool) error {
		if module != "all" && module != name {
			return nil
		}
		if !configured {
			if module == name {
				return fmt.Errorf("modulo %q non configurato in %s", name, *configPath)
			}
			return nil
		}
		selected = append(selected, build())
		return nil
	}
	if err := add("certs", func() engine.Check { return certs.New(*cfg.Checks.Certs) }, cfg.Checks.Certs != nil); err != nil {
		return err
	}
	if err := add("http", func() engine.Check { return httpcheck.New(*cfg.Checks.HTTP) }, cfg.Checks.HTTP != nil); err != nil {
		return err
	}
	if err := add("nats", func() engine.Check { return nats.New(*cfg.Checks.NATS) }, cfg.Checks.NATS != nil); err != nil {
		return err
	}
	if err := add("haproxy", func() engine.Check { return haproxy.New(*cfg.Checks.HAProxy) }, cfg.Checks.HAProxy != nil); err != nil {
		return err
	}
	if err := add("stream", func() engine.Check { return stream.New(*cfg.Checks.Stream) }, cfg.Checks.Stream != nil); err != nil {
		return err
	}
	if len(selected) == 0 {
		return fmt.Errorf("nessun modulo selezionato (modulo %q sconosciuto o niente configurato)", module)
	}

	res := engine.Run(context.Background(), selected, time.Duration(cfg.TimeoutSeconds)*time.Second)

	switch *format {
	case "text":
		fmt.Print(output.Text(res))
	case "markdown":
		fmt.Print(output.Markdown(res, module))
	case "json":
		s, err := output.JSON(res)
		if err != nil {
			return err
		}
		fmt.Println(s)
	default:
		return fmt.Errorf("formato %q sconosciuto", *format)
	}

	if *exitOnBad {
		worst := engine.Worst(res.Findings)
		if worst == engine.BAD || worst == engine.ERROR {
			os.Exit(2)
		}
	}
	return nil
}
