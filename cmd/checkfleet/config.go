// Turning flags and config into the values the engine needs.

package main

import (
	"strings"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// commaSet parses a comma-separated flag into a set (nil when empty).
func commaSet(s string) map[string]bool {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	set := map[string]bool{}
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			set[p] = true
		}
	}
	return set
}

// runOptions builds the engine run options from the config.
func runOptions(cfg *engine.Config) engine.Options {
	return engine.Options{
		Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second,
		Retries: cfg.Retries,
		Backoff: time.Duration(cfg.RetryBackoffMS) * time.Millisecond,
	}
}

// effectiveConcurrency resolves the global concurrency cap: a --max-concurrency
// flag (>= 0) wins over the config's max_concurrency (CF-116). -1 means the flag
// was not set. 0 = unbounded.
func effectiveConcurrency(flag int, cfg *engine.Config) int {
	if flag >= 0 {
		return flag
	}
	return cfg.MaxConcurrency
}

// loadConfig loads the base config, overlaying stack profiles when set. --stack
// accepts a comma-separated list applied left-to-right (last wins), e.g.
// --stack region,env (CF-117).
func loadConfig(path, stack string) (*engine.Config, error) {
	stacks := splitStacks(stack)
	if len(stacks) == 0 {
		return engine.LoadConfig(path)
	}
	return engine.LoadConfigStacks(path, stacks)
}

// splitStacks parses a comma-separated --stack value into trimmed, non-empty
// names, preserving order.
func splitStacks(stack string) []string { return splitCSV(stack) }

// splitCSV splits a comma-separated flag value into trimmed, non-empty entries,
// preserving order (used for --stack and the multi-sink --output).
func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
