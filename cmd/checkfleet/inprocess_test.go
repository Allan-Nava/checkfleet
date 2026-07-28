package main

// In-process tests for the CLI surface (CF-157).
//
// The end-to-end tests elsewhere in this package exec the built binary, which is
// the right shape for asserting exit codes — but it means none of that code is
// *measured*, and the CLI is where the flags, the gate and the fan-out live:
// everything the compatibility contract promises not to break. These tests call
// the functions directly instead, so the contractual paths are both exercised and
// visible in coverage.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/baseline"
	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/history"
)

// sampleResult is a run with one of each status and one numeric metric.
func sampleResult() engine.Result {
	v := 12.5
	return engine.Result{
		Findings: []engine.Finding{
			{Check: "http", Target: "https://a.example", Status: engine.BAD, Message: "HTTP 500 (want 200)"},
			{Check: "tcp", Target: "db:5432", Status: engine.OK, Message: "connected", Value: &v, Unit: "ms"},
		},
		Started:  time.Unix(1700000000, 0).UTC(),
		Duration: 25 * time.Millisecond,
	}
}

// offlineConfig writes a config whose targets can never reach anything: a closed
// loopback port. Deterministic, and it never touches the network.
func offlineConfig(t *testing.T) string {
	t.Helper()
	return writeCfg(t, `timeout_seconds: 1
checks:
  tcp:
    targets:
      - {name: closed, address: 127.0.0.1:1}
`)
}

// --- render -----------------------------------------------------------------

func TestRenderEveryFormat(t *testing.T) {
	res := sampleResult()
	for _, format := range []string{"text", "markdown", "json", "junit", "html", "prometheus", "otlp", "csv", "sarif"} {
		out, err := render(format, res, renderCtx{module: "all", configPath: "checkfleet.yml"})
		if err != nil {
			t.Errorf("%s: %v", format, err)
			continue
		}
		if out == "" {
			t.Errorf("%s: rendered nothing", format)
		}
		// Every format has to carry the finding that matters, in some form.
		if !strings.Contains(out, "a.example") {
			t.Errorf("%s: the BAD target is missing from the output:\n%s", format, out)
		}
	}
}

func TestRenderUnknownFormat(t *testing.T) {
	if _, err := render("yaml", sampleResult(), renderCtx{}); err == nil {
		t.Error("an unknown format must be an error, not empty output")
	}
}

func TestRenderTextColorOnlyWhenAsked(t *testing.T) {
	const esc = "\x1b["
	plain, _ := render("text", sampleResult(), renderCtx{})
	if strings.Contains(plain, esc) {
		t.Error("text output must have no ANSI escapes unless colour was requested")
	}
	coloured, _ := render("text", sampleResult(), renderCtx{color: true})
	if !strings.Contains(coloured, esc) {
		t.Error("colour was requested but no ANSI escapes were emitted")
	}
}

// --- writing output ---------------------------------------------------------

func TestAtomicWriteReplacesWholeFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.json")
	if err := atomicWrite(path, "first"); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(path, "second"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "second" {
		t.Errorf("--out-file must be replaced, not appended: %q", b)
	}
}

// $GITHUB_STEP_SUMMARY is shared with the other steps of the same job, so this
// one must append. A rename would throw their contributions away.
func TestAppendFileKeepsExistingContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "summary.md")
	if err := os.WriteFile(path, []byte("from another step\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := appendFile(path, "from checkfleet\n"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "from another step") || !strings.Contains(string(b), "from checkfleet") {
		t.Errorf("append lost content: %q", b)
	}
}

// --- sinks ------------------------------------------------------------------

// recorder is a fake webhook capturing the last payload it received.
func recorder(t *testing.T, status int) (*httptest.Server, *string) {
	t.Helper()
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, r.ContentLength)
		if r.ContentLength > 0 {
			_, _ = r.Body.Read(b)
		}
		got = string(b)
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func TestEmitPushSinks(t *testing.T) {
	for _, sink := range []string{"slack", "discord", "teams", "webhook"} {
		t.Run(sink, func(t *testing.T) {
			srv, got := recorder(t, http.StatusOK)
			t.Setenv("TEST_HOOK", srv.URL)
			if err := emit(sink, sampleResult(), sinkOptions{renderCtx: renderCtx{module: "all"}, webhookEnv: "TEST_HOOK"}); err != nil {
				t.Fatalf("%s: %v", sink, err)
			}
			if *got == "" {
				t.Fatalf("%s: the webhook received nothing", sink)
			}
			if !json.Valid([]byte(*got)) {
				t.Errorf("%s: payload is not valid JSON: %s", sink, *got)
			}
		})
	}
}

// A sink whose env var is unset must say *which* variable, since that is the
// entire fix. It must never fall back to a URL from the config or the flags:
// webhook URLs are secrets and stay in the environment.
func TestEmitPushSinkUnsetEnv(t *testing.T) {
	for _, sink := range []string{"slack", "discord", "teams", "webhook", "telegram"} {
		t.Setenv("EMPTY_HOOK", "")
		err := emit(sink, sampleResult(), sinkOptions{
			renderCtx: renderCtx{module: "all"}, webhookEnv: "EMPTY_HOOK",
			tgTokenEnv: "EMPTY_HOOK", tgChatEnv: "EMPTY_HOOK",
		})
		if err == nil {
			t.Errorf("%s: an unset webhook env must be an error", sink)
			continue
		}
		if !strings.Contains(err.Error(), "EMPTY_HOOK") {
			t.Errorf("%s: the error must name the env var, got %v", sink, err)
		}
	}
}

// Telegram's endpoint is the real api.telegram.org, so there is nothing here to
// point at a fixture: the send path is deliberately left to the unset-env case
// above and to internal/output's renderer tests. A test that reached the real API
// would be a bug, not better coverage.

func TestEmitWebhookWithTemplate(t *testing.T) {
	srv, got := recorder(t, http.StatusOK)
	t.Setenv("TEST_HOOK", srv.URL)
	tmpl := filepath.Join(t.TempDir(), "payload.tmpl")
	if err := os.WriteFile(tmpl, []byte(`worst={{.Worst}} total={{.Total}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := emit("webhook", sampleResult(), sinkOptions{
		renderCtx: renderCtx{module: "all"}, webhookEnv: "TEST_HOOK", tmplFile: tmpl,
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(*got, "worst=BAD") {
		t.Errorf("the template was not applied: %q", *got)
	}
}

func TestEmitWebhookMissingTemplate(t *testing.T) {
	t.Setenv("TEST_HOOK", "http://127.0.0.1:1")
	err := emit("webhook", sampleResult(), sinkOptions{
		renderCtx: renderCtx{module: "all"}, webhookEnv: "TEST_HOOK",
		tmplFile: filepath.Join(t.TempDir(), "nope.tmpl"),
	})
	if err == nil || !strings.Contains(err.Error(), "nope.tmpl") {
		t.Errorf("a missing template must name the file, got %v", err)
	}
}

func TestEmitGitHubWritesJobSummary(t *testing.T) {
	summary := filepath.Join(t.TempDir(), "summary.md")
	t.Setenv("GITHUB_STEP_SUMMARY", summary)
	if err := emit("github", sampleResult(), sinkOptions{renderCtx: renderCtx{module: "all"}}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(summary)
	if err != nil {
		t.Fatalf("no job summary written: %v", err)
	}
	if !strings.Contains(string(b), "a.example") {
		t.Errorf("the summary must contain the findings:\n%s", b)
	}
}

// Outside Actions there is nowhere to put a summary; the annotations on stdout
// are still useful, so this must not be an error.
func TestEmitGitHubWithoutSummaryFile(t *testing.T) {
	t.Setenv("GITHUB_STEP_SUMMARY", "")
	if err := emit("github", sampleResult(), sinkOptions{renderCtx: renderCtx{module: "all"}}); err != nil {
		t.Errorf("running outside Actions must not fail: %v", err)
	}
}

func TestEmitFormatToOutFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.json")
	if err := emit("json", sampleResult(), sinkOptions{renderCtx: renderCtx{module: "all"}, outFile: path}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("--out-file must contain valid JSON: %v", err)
	}
	if doc["worst"] != "BAD" {
		t.Errorf("worst missing from the written report: %v", doc)
	}
}

// Fan-out isolation is a contract: a dead webhook must not fail the run or stop
// the other sinks. The finding gate is a separate decision, and losing a Slack
// notification must not turn a healthy fleet into a red build.
func TestEmitAllIsolatesFailingSink(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.json")
	t.Setenv("EMPTY_HOOK", "")
	err := emitAll([]string{"slack", "json"}, sampleResult(), sinkOptions{
		renderCtx: renderCtx{module: "all"}, webhookEnv: "EMPTY_HOOK", outFile: path,
	})
	if err != nil {
		t.Fatalf("fan-out must not fail the run: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("the healthy sink must still have been written: %v", err)
	}
}

// With a single sink the error does abort, so a misconfigured one-sink pipeline
// fails loudly instead of silently sending nothing.
func TestEmitAllSingleSinkFails(t *testing.T) {
	t.Setenv("EMPTY_HOOK", "")
	if err := emitAll([]string{"slack"}, sampleResult(), sinkOptions{webhookEnv: "EMPTY_HOOK"}); err == nil {
		t.Error("a single failing sink must abort the command")
	}
}

func TestPostJSONRejectsNon2xx(t *testing.T) {
	srv, _ := recorder(t, http.StatusInternalServerError)
	err := postJSON(t.Context(), srv.URL, `{}`)
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Errorf("a 500 from the webhook must be reported with its code, got %v", err)
	}
}

func TestPingDeadmanUsesFailPathOnBad(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
	}))
	defer srv.Close()

	if err := pingDeadman(t.Context(), srv.URL+"/hc", engine.OK); err != nil {
		t.Fatal(err)
	}
	if err := pingDeadman(t.Context(), srv.URL+"/hc", engine.BAD); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[0] != "/hc" || paths[1] != "/hc/fail" {
		t.Errorf("want [/hc /hc/fail], got %v", paths)
	}
}

// --- baseline ---------------------------------------------------------------

func TestApplyBaselineNoFileIsInert(t *testing.T) {
	res := sampleResult()
	gated, skip, err := applyBaseline("", false, false, res.Findings)
	if err != nil || skip || len(gated) != len(res.Findings) {
		t.Errorf("without --baseline nothing changes: %d findings, skip=%v, %v", len(gated), skip, err)
	}
}

// The adoption flow: the run that records the debt must not fail the build, or
// the gate gets disabled on day one and protects nothing after that.
func TestApplyBaselineFirstRunSkipsGate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")
	_, skip, err := applyBaseline(path, false, true, sampleResult().Findings)
	if err != nil {
		t.Fatal(err)
	}
	if !skip {
		t.Error("the run that records the baseline must skip the gate")
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("the baseline file was not written: %v", err)
	}
}

func TestApplyBaselineGatesOnlyNewFindings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")
	known := sampleResult().Findings
	if err := baseline.Save(path, known, time.Now()); err != nil {
		t.Fatal(err)
	}

	// Same findings plus one that is new.
	current := append([]engine.Finding{}, known...)
	current = append(current, engine.Finding{Check: "dns", Target: "new.example", Status: engine.BAD, Message: "NXDOMAIN"})

	gated, skip, err := applyBaseline(path, false, true, current)
	if err != nil || skip {
		t.Fatalf("unexpected: skip=%v err=%v", skip, err)
	}
	if len(gated) != 1 || gated[0].Target != "new.example" {
		t.Errorf("only the new finding should reach the gate, got %+v", gated)
	}
}

// --baseline without --fail-on-new must be inert: adding a baseline to a
// pipeline can never quietly loosen a gate that was already there.
func TestApplyBaselineWithoutFailOnNewIsInert(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")
	if err := baseline.Save(path, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	gated, _, err := applyBaseline(path, false, false, sampleResult().Findings)
	if err != nil {
		t.Fatal(err)
	}
	if len(gated) != 2 {
		t.Errorf("the gate must still see every finding, got %d", len(gated))
	}
}

// --- history ----------------------------------------------------------------

func TestRecordHistoryKeepsMetrics(t *testing.T) {
	path := filepath.Join(t.TempDir(), "h.jsonl")
	if _, err := recordHistory(path, sampleResult(), 3, 20); err != nil {
		t.Fatal(err)
	}
	recs, err := history.Open(path).Recent(0)
	if err != nil || len(recs) != 1 {
		t.Fatalf("want one record, got %d (%v)", len(recs), err)
	}
	var withValue int
	for _, e := range recs[0].Entries {
		if e.Value != nil {
			withValue++
			if e.Unit != "ms" {
				t.Errorf("unit lost: %+v", e)
			}
		}
	}
	// Without this the "metric over time" charts and every history-derived
	// insight are empty for CLI-recorded runs.
	if withValue != 1 {
		t.Errorf("the numeric metric must be persisted, got %d entries with a value", withValue)
	}
}

func TestRecordHistoryReportsFlapping(t *testing.T) {
	path := filepath.Join(t.TempDir(), "h.jsonl")
	flip := []engine.Status{engine.OK, engine.BAD, engine.OK, engine.BAD}
	var flaps []engine.Finding
	for i, st := range flip {
		res := engine.Result{
			Findings: []engine.Finding{{Check: "http", Target: "a", Status: st}},
			Started:  time.Unix(int64(1700000000+i), 0),
		}
		var err error
		flaps, err = recordHistory(path, res, 3, 20)
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(flaps) != 1 || flaps[0].Check != "flap" || flaps[0].Status != engine.WARN {
		t.Fatalf("three transitions should raise one WARN flap finding, got %+v", flaps)
	}
	if !strings.Contains(flaps[0].Message, "3 state changes") {
		t.Errorf("the message should count the transitions: %q", flaps[0].Message)
	}
}

// --- flags and config -------------------------------------------------------

func TestCommaSet(t *testing.T) {
	if commaSet("") != nil {
		t.Error("an empty flag must mean no filter, not an empty filter")
	}
	got := commaSet(" http , certs ,, ")
	if len(got) != 2 || !got["http"] || !got["certs"] {
		t.Errorf("whitespace and empty entries should be handled: %v", got)
	}
}

func TestSplitCSVAndStacks(t *testing.T) {
	if got := splitCSV("a, b ,,c"); len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Errorf("splitCSV: %v", got)
	}
	if got := splitStacks(""); len(got) != 0 {
		t.Errorf("no --stack means no overlay: %v", got)
	}
	// Order matters: overlays apply left to right, last wins.
	if got := splitStacks("region,env"); len(got) != 2 || got[0] != "region" {
		t.Errorf("stack order must be preserved: %v", got)
	}
}

func TestEffectiveConcurrency(t *testing.T) {
	cfg := &engine.Config{MaxConcurrency: 8}
	if got := effectiveConcurrency(-1, cfg); got != 8 {
		t.Errorf("unset flag must fall back to the config: %d", got)
	}
	if got := effectiveConcurrency(2, cfg); got != 2 {
		t.Errorf("the flag must win: %d", got)
	}
	if got := effectiveConcurrency(0, cfg); got != 0 {
		t.Errorf("an explicit 0 means unbounded and must beat the config: %d", got)
	}
}

func TestRunOptionsFromConfig(t *testing.T) {
	opts := runOptions(&engine.Config{TimeoutSeconds: 7, Retries: 2, RetryBackoffMS: 250})
	if opts.Timeout != 7*time.Second || opts.Retries != 2 {
		t.Errorf("options not derived from the config: %+v", opts)
	}
}

func TestLoadConfigWithStackOverlay(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "checkfleet.yml")
	if err := os.WriteFile(base, []byte("timeout_seconds: 5\nchecks:\n  tcp:\n    targets:\n      - {name: a, address: a:1}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "checkfleet.prod.yml"), []byte("timeout_seconds: 30\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadConfig(base, "prod")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TimeoutSeconds != 30 {
		t.Errorf("the overlay must win: %d", cfg.TimeoutSeconds)
	}
	if cfg.Checks.TCP == nil {
		t.Error("the base module must survive an overlay that does not mention it")
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	if _, err := loadConfig(filepath.Join(t.TempDir(), "nope.yml"), ""); err == nil {
		t.Error("a missing config must be an error — a run that checks nothing must never look successful")
	}
}

func TestNewLoggerFormats(t *testing.T) {
	if newLogger("json") == nil || newLogger("text") == nil || newLogger("nonsense") == nil {
		t.Error("newLogger must always return a usable logger")
	}
}

// --- commands, in process ---------------------------------------------------

func TestRunCheckInProcess(t *testing.T) {
	cfg := offlineConfig(t)
	out := filepath.Join(t.TempDir(), "report.json")
	// No --exit-on, so the gate is off and this cannot call os.Exit.
	if err := runCheck([]string{"all", "--config", cfg, "--output", "json", "--out-file", out}); err != nil {
		t.Fatalf("a run whose target is down is still a successful run: %v", err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Schema int    `json:"schema"`
		Worst  string `json:"worst"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Schema != 1 || doc.Worst != "ERROR" {
		t.Errorf("unreachable target must be ERROR (not BAD) with schema 1, got %+v", doc)
	}
}

func TestRunCheckFilters(t *testing.T) {
	cfg := offlineConfig(t)
	for _, args := range [][]string{
		{"all", "--config", cfg, "--only", "tcp", "--output", "csv"},
		{"all", "--config", cfg, "--min-severity", "warn", "--output", "csv"},
		{"all", "--config", cfg, "--target", "clos*", "--output", "csv"},
		{"tcp", "--config", cfg, "--output", "text", "--no-color"},
		{"all", "--config", cfg, "--max-concurrency", "1", "--output", "text"},
	} {
		if err := runCheck(args); err != nil {
			t.Errorf("%v: %v", args, err)
		}
	}
}

func TestRunCheckSystemicErrors(t *testing.T) {
	cfg := offlineConfig(t)
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"unknown module", []string{"nope", "--config", cfg}, "nope"},
		{"missing config", []string{"all", "--config", filepath.Join(t.TempDir(), "nope.yml")}, "nope.yml"},
		{"unknown format", []string{"all", "--config", cfg, "--output", "yaml"}, "yaml"},
		{"diff without history", []string{"all", "--config", cfg, "--diff"}, "--history"},
		{"fail-on-new without baseline", []string{"all", "--config", cfg, "--fail-on-new"}, "--baseline"},
		{"bad exit-on", []string{"all", "--config", cfg, "--exit-on", "catastrophe"}, "catastrophe"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := runCheck(c.args)
			if err == nil {
				t.Fatal("want a systemic error (exit 1), got nil")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("the error must mention %q, got %v", c.want, err)
			}
		})
	}
}

func TestRunCheckHistoryAndDiff(t *testing.T) {
	cfg := offlineConfig(t)
	hist := filepath.Join(t.TempDir(), "h.jsonl")
	for range 2 {
		if err := runCheck([]string{"all", "--config", cfg, "--history", hist, "--output", "csv"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := runCheck([]string{"all", "--config", cfg, "--history", hist, "--diff"}); err != nil {
		t.Fatalf("--diff with a history must work: %v", err)
	}
	recs, err := history.Open(hist).Recent(0)
	if err != nil || len(recs) < 2 {
		t.Errorf("both runs should be recorded, got %d (%v)", len(recs), err)
	}
}

func TestRunValidateAcceptsGoodConfig(t *testing.T) {
	// A valid config returns nil; an invalid one calls os.Exit(1), which is
	// covered by the end-to-end tests instead.
	if err := runValidate([]string{"--config", offlineConfig(t)}); err != nil {
		t.Errorf("a valid config must validate: %v", err)
	}
}

func TestDiagnosticCommandsInProcess(t *testing.T) {
	cfg := offlineConfig(t)
	cases := [][]string{
		{"targets", "--config", cfg, "--output", "json"},
		{"doctor", "--config", cfg, "--no-probe", "--output", "json"},
		{"explain", "tcp"},
		{"completion", "bash"},
	}
	run := map[string]func([]string) error{
		"targets": runTargets, "doctor": runDoctor, "explain": runExplain, "completion": runCompletion,
	}
	for _, args := range cases {
		// Diagnostic commands never gate: they exit 0 unless something systemic
		// went wrong (the M31 rule).
		if err := run[args[0]](args[1:]); err != nil {
			t.Errorf("%v: %v", args, err)
		}
	}
}

func TestRunInitInProcess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "checkfleet.yml")
	if err := runInit([]string{"--config", path, "--modules", "tcp,certs"}); err != nil {
		t.Fatal(err)
	}
	// It must not clobber an existing config without --force: that file is the
	// user's description of their fleet.
	if err := runInit([]string{"--config", path, "--modules", "tcp"}); err == nil {
		t.Error("init must refuse to overwrite without --force")
	}
	if err := runInit([]string{"--config", path, "--modules", "tcp", "--force"}); err != nil {
		t.Errorf("--force must allow the overwrite: %v", err)
	}
	// Whatever it wrote has to be a config checkfleet itself accepts.
	if err := runValidate([]string{"--config", path}); err != nil {
		t.Errorf("a scaffolded config must validate: %v", err)
	}
}

func TestWarnUnknownKeysWritesNotice(t *testing.T) {
	cfg := writeCfg(t, "checks:\n  postgress: {}\n")
	var sb strings.Builder
	warnUnknownKeys(&sb, cfg, "")
	if !strings.Contains(sb.String(), "postgress") || !strings.Contains(sb.String(), "postgres") {
		t.Errorf("want the key named and the fix suggested, got %q", sb.String())
	}
	var clean strings.Builder
	warnUnknownKeys(&clean, offlineConfig(t), "")
	if clean.String() != "" {
		t.Errorf("a valid config must be silent, got %q", clean.String())
	}
}

func TestWatchFrameRendersHeader(t *testing.T) {
	frame := watchFrame(sampleResult(), time.Unix(1700000000, 0).UTC(), 5*time.Second, false)
	if !strings.Contains(frame, "a.example") {
		t.Errorf("the frame must contain the findings:\n%s", frame)
	}
	// The header is what makes a live view readable: it must say when this frame
	// was drawn and how long until the next one.
	if !strings.Contains(frame, "5s") {
		t.Errorf("the frame header should show the interval:\n%s", frame)
	}
}
