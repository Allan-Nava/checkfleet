// checkfleet: a fleet of infrastructure checks with one binary.
//
//	checkfleet check all   --config checkfleet.yml [--output text|markdown|json]
//	checkfleet check certs --config checkfleet.yml
//	checkfleet version
//
// Exit code: 0 also on WARN/BAD findings (a check that ran IS a success —
// gate on the output instead, or pass --exit-on warn|bad|error to get exit 2,
// or --exit-code N, when a finding reaches that severity). Non-zero otherwise
// only for systemic errors (unreadable config, unknown module, bad flag).
package main

import (
	"fmt"
	"os"
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
	case "init":
		if err := runInit(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "checkfleet:", err)
			os.Exit(1)
		}
	case "check":
		if err := runCheck(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "checkfleet:", err)
			os.Exit(1)
		}
	case "serve":
		if err := runServe(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "checkfleet:", err)
			os.Exit(1)
		}
	case "validate":
		if err := runValidate(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "checkfleet:", err)
			os.Exit(1)
		}
	case "report-issues":
		if err := runReportIssues(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "checkfleet:", err)
			os.Exit(1)
		}
	case "alert":
		if err := runAlert(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "checkfleet:", err)
			os.Exit(1)
		}
	case "doctor":
		if err := runDoctor(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "checkfleet:", err)
			os.Exit(1)
		}
	case "targets":
		if err := runTargets(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "checkfleet:", err)
			os.Exit(1)
		}
	case "explain":
		if err := runExplain(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "checkfleet:", err)
			os.Exit(1)
		}
	case "completion":
		if err := runCompletion(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "checkfleet:", err)
			os.Exit(1)
		}
	case "perms":
		if err := runPerms(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "checkfleet:", err)
			os.Exit(1)
		}
	case "insight":
		if err := runInsight(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "checkfleet:", err)
			os.Exit(1)
		}
	case "skill":
		if err := runSkill(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "checkfleet:", err)
			os.Exit(1)
		}
	default:
		usage()
		os.Exit(64)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  checkfleet init [--modules certs,http] [--config checkfleet.yml] [--force]     # scaffold a starter config
  checkfleet check <all|certs|http|nats|haproxy|stream|patroni|consul|postgres|dns|redis|keycloak|tcp|tls|ntp|rabbitmq|grpc|ldap|kafka|ingest|s3|smtp|elasticsearch|mongodb|mysql|etcd|clickhouse|vault|memcached|cassandra> --config checkfleet.yml [--output text|markdown|json|junit|html|github|sarif|prometheus|otlp|csv|slack|discord|teams|telegram|webhook,...] [--out-file PATH] [--no-color] [--only ...] [--min-severity warn] [--target glob] [--watch 5s] [--history F --diff] [--max-concurrency N] [--baseline F --fail-on-new] [--exit-on warn|bad|error] [--exit-code N]
  checkfleet serve --config checkfleet.yml [--listen :9876] [--interval 60s] [--max-concurrency N]   # export Prometheus metrics
  checkfleet report-issues --config checkfleet.yml [--forge github|gitlab]     # open/close tracker issues from BAD findings
  checkfleet alert --config checkfleet.yml --provider pagerduty --key-env K    # create/resolve on-call alerts from BAD/ERROR
  checkfleet validate --config checkfleet.yml                                  # validate the config without running the checks
  checkfleet doctor --config checkfleet.yml [--output text|json] [--no-probe]   # preflight: unset ${ENV}, bad targets, unreachable hosts
  checkfleet targets --config checkfleet.yml [--output text|json] [--against hosts.ini [--group web]] [--module certs]   # what is covered
  checkfleet perms [module] [--config checkfleet.yml] [--output text|markdown|json]   # least privilege each check needs
  checkfleet explain [module]                                                 # what a module checks and its thresholds
  checkfleet completion <bash|zsh|fish>                                        # print a shell completion script
  checkfleet insight --history F [--digest] [--score] [--clusters] [--anomaly] [--recovery] [--forecast --threshold N] [--slo 0.999] [--output text|json]   # what the history implies
  checkfleet skill <install|print> [--dir PATH]                                # install the agent skill shipped in this binary
  checkfleet version`)
}
