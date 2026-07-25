package main

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// startTCP starts a throwaway TCP listener that accepts and immediately closes
// connections, so the tcp check sees a reachable target. Returns host:port.
func startTCP(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	return ln.Addr().String()
}

func writeConfig(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return p
}

// RunChecks against a reachable tcp target must succeed, report the module and a
// finding, and the exports must render the cached result.
func TestRunChecks_TCP_OK(t *testing.T) {
	addr := startTCP(t)
	cfg := writeConfig(t, "checkfleet.yml",
		"timeout_seconds: 5\nchecks:\n  tcp:\n    targets:\n      - address: \""+addr+"\"\n")

	app := NewApp("test")
	rep := app.RunChecks(cfg, "")

	if rep.Err != "" {
		t.Fatalf("unexpected error: %s", rep.Err)
	}
	if len(rep.Findings) == 0 {
		t.Fatal("expected at least one finding")
	}
	if !contains(rep.Modules, "tcp") {
		t.Fatalf("modules = %v, want it to include tcp", rep.Modules)
	}
	if rep.Worst != "OK" {
		t.Fatalf("worst = %q, want OK for a reachable target", rep.Worst)
	}
	if rep.OK < 1 {
		t.Fatalf("ok count = %d, want >= 1", rep.OK)
	}

	// Exports render the cached run.
	if md := app.ExportMarkdown(); !strings.Contains(md, "tcp") {
		t.Fatalf("markdown export missing the tcp finding:\n%s", md)
	}
	js, err := app.ExportJSON()
	if err != nil {
		t.Fatalf("json export: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(js), &payload); err != nil {
		t.Fatalf("json export is not valid JSON: %v", err)
	}
}

func TestRunChecks_Errors(t *testing.T) {
	app := NewApp("test")

	if rep := app.RunChecks("", ""); rep.Err == "" {
		t.Fatal("empty config path should report an error")
	}
	if rep := app.RunChecks("/no/such/checkfleet.yml", ""); rep.Err == "" {
		t.Fatal("missing config file should report an error")
	}
	// A valid file with no modules configured is an error too.
	empty := writeConfig(t, "checkfleet.yml", "timeout_seconds: 5\nchecks: {}\n")
	if rep := app.RunChecks(empty, ""); rep.Err == "" {
		t.Fatal("config with no modules should report an error")
	}
}

func TestListStacks(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"checkfleet.yml", "checkfleet.prod.yml", "checkfleet.edge.yaml", "unrelated.yml"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("checks: {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	app := NewApp("test")
	got := app.ListStacks(filepath.Join(dir, "checkfleet.yml"))
	// Sorted, base excluded, unrelated.yml ignored.
	if len(got) != 2 || got[0] != "edge" || got[1] != "prod" {
		t.Fatalf("ListStacks = %v, want [edge prod]", got)
	}
	if app.ListStacks("") != nil {
		t.Fatal("ListStacks(\"\") should be nil")
	}
}

func TestDefaultConfigPath(t *testing.T) {
	app := NewApp("test")

	dir := t.TempDir()
	t.Chdir(dir)
	if got := app.DefaultConfigPath(); got != "" {
		t.Fatalf("no checkfleet.yml present, got %q", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "checkfleet.yml"), []byte("checks: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := app.DefaultConfigPath(); got == "" {
		t.Fatal("checkfleet.yml present but DefaultConfigPath returned empty")
	}
}

func TestStartupConfig(t *testing.T) {
	app := NewApp("test")

	// No env, no ./checkfleet.yml → a starter config is created under the (temp)
	// user config dir and returned, with no auto-run.
	tmp := t.TempDir()
	t.Chdir(tmp)
	t.Setenv("HOME", tmp)            // darwin user config dir
	t.Setenv("XDG_CONFIG_HOME", tmp) // linux user config dir
	t.Setenv("CHECKFLEET_CONFIG", "")
	t.Setenv("CHECKFLEET_AUTORUN", "")
	if s := app.StartupConfig(); s.Path == "" || !s.Created || s.AutoRun {
		t.Fatalf("StartupConfig = %+v, want a created starter path and no autorun", s)
	}

	// Env-chosen path + auto-run.
	t.Setenv("CHECKFLEET_CONFIG", "/etc/checkfleet.yml")
	t.Setenv("CHECKFLEET_AUTORUN", "1")
	if s := app.StartupConfig(); s.Path != "/etc/checkfleet.yml" || !s.AutoRun {
		t.Fatalf("StartupConfig = %+v, want path set and autoRun true", s)
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func TestRenderReportFormats(t *testing.T) {
	res := engine.Result{Findings: []engine.Finding{
		{Check: "http", Target: "x", Status: engine.BAD, Message: "500"},
	}}
	cases := map[string]string{
		"markdown": "md", "json": "json", "html": "html",
		"junit": "xml", "prometheus": "prom", "otlp": "json",
	}
	for format, wantExt := range cases {
		content, ext, err := renderReport(res, "all", format)
		if err != nil {
			t.Errorf("%s: %v", format, err)
		}
		if ext != wantExt {
			t.Errorf("%s: ext = %q, want %q", format, ext, wantExt)
		}
		if content == "" {
			t.Errorf("%s: empty content", format)
		}
	}
}

func TestExplainAndValidate(t *testing.T) {
	app := NewApp("test")
	if app.Explain("certs") == "" {
		t.Error("Explain(certs) should not be empty")
	}
	if app.Explain("nope") != "" {
		t.Error("Explain(unknown) should be empty")
	}
	valid := writeConfig(t, "checkfleet.yml",
		"checks:\n  certs:\n    warn_days: 30\n    crit_days: 7\n    targets: [\"x:443\"]\n")
	if p := app.Validate(valid, ""); len(p) != 0 {
		t.Errorf("valid config: want no problems, got %v", p)
	}
	if p := app.Validate(writeConfig(t, "checkfleet.yml", "checks: {}\n"), ""); len(p) == 0 {
		t.Error("config with no modules should report problems")
	}
}

func TestRunChecksDiff(t *testing.T) {
	addr := startTCP(t)
	cfg := writeConfig(t, "checkfleet.yml",
		"timeout_seconds: 5\nchecks:\n  tcp:\n    targets:\n      - address: \""+addr+"\"\n")
	app := NewApp("test")

	first := app.RunChecks(cfg, "")
	if len(first.Changes) != 0 {
		t.Errorf("first run should have no changes, got %v", first.Changes)
	}
	second := app.RunChecks(cfg, "")
	if len(second.Changes) != 0 {
		t.Errorf("stable OK->OK run should have no changes, got %v", second.Changes)
	}
}

func TestConfigEditorBindings(t *testing.T) {
	app := NewApp("test")
	dir := t.TempDir()
	path := dir + "/checkfleet.yml"

	// ReadConfig on a missing file returns empty, no error.
	if s, err := app.ReadConfig(path); err != nil || s != "" {
		t.Fatalf("ReadConfig(missing): got %q, %v", s, err)
	}

	body := "checks:\n  certs:\n    targets: [\"x:443\"]\n"
	if err := app.SaveConfig(path, body); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	if s, err := app.ReadConfig(path); err != nil || s != body {
		t.Fatalf("ReadConfig after save: got %q, %v", s, err)
	}

	// SaveConfig with an empty path is rejected.
	if err := app.SaveConfig("  ", body); err == nil {
		t.Error("SaveConfig(empty path) should error")
	}

	// ValidateText: valid text has no problems, malformed text reports one.
	if p := app.ValidateText(body); len(p) != 0 {
		t.Errorf("ValidateText(valid): want none, got %v", p)
	}
	if p := app.ValidateText("checks: [::bad"); len(p) == 0 {
		t.Error("ValidateText(malformed) should report a problem")
	}
	if p := app.ValidateText("checks: {}\n"); len(p) == 0 {
		t.Error("ValidateText(no modules) should report a problem")
	}
}

func TestAddEndpointBinding(t *testing.T) {
	app := NewApp("test")
	out, err := app.AddEndpoint("", "http", "https://example.com/health", "", 200)
	if err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}
	if !strings.Contains(out, "https://example.com/health") || !strings.Contains(out, "expect_status: 200") {
		t.Errorf("endpoint not written into YAML:\n%s", out)
	}
	// ValidateText should accept the freshly built config.
	if p := app.ValidateText(out); len(p) != 0 {
		t.Errorf("built config should validate, got %v", p)
	}
	if _, err := app.AddEndpoint("", "bogus", "x", "", 0); err == nil {
		t.Error("unsupported kind should error")
	}
}

func TestScheduleSnippet(t *testing.T) {
	s := scheduleSnippet("/etc/checkfleet/checkfleet.yml", "5m")
	if !strings.Contains(s, "*/5 * * * *") {
		t.Errorf("cron minutes wrong: %s", s)
	}
	if !strings.Contains(s, "serve --config /etc/checkfleet/checkfleet.yml --interval 5m") {
		t.Errorf("serve line missing: %s", s)
	}
	// Sub-minute intervals clamp to 1 minute for cron.
	if got := intervalMinutes("30s"); got != 1 {
		t.Errorf("intervalMinutes(30s) = %d, want 1", got)
	}
	if got := intervalMinutes("2h"); got != 120 {
		t.Errorf("intervalMinutes(2h) = %d, want 120", got)
	}
	// Empty path falls back to a relative default.
	if !strings.Contains(scheduleSnippet("", ""), "checkfleet.yml") {
		t.Error("empty path should fall back to checkfleet.yml")
	}
}

func TestTrendPersistsAcrossRuns(t *testing.T) {
	addr := startTCP(t)
	dir := t.TempDir()
	cfg := dir + "/checkfleet.yml"
	if err := os.WriteFile(cfg,
		[]byte("timeout_seconds: 5\nchecks:\n  tcp:\n    targets:\n      - address: \""+addr+"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := NewApp("test")
	app.RunChecks(cfg, "")
	app.RunChecks(cfg, "")

	points, err := app.Trend(cfg, 10)
	if err != nil {
		t.Fatalf("Trend: %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("want 2 persisted runs, got %d", len(points))
	}
	if points[0].Worst != "OK" || points[0].OK != 1 {
		t.Errorf("unexpected trend point: %+v", points[0])
	}

	// A fresh App reading the same file still sees the history (survives restart).
	if again, _ := NewApp("test").Trend(cfg, 10); len(again) != 2 {
		t.Errorf("history should survive a new App instance, got %d", len(again))
	}
}

func TestTrendNoConfigNoError(t *testing.T) {
	if pts, err := NewApp("test").Trend("", 10); err != nil || pts != nil {
		t.Errorf("empty config path: want nil,nil got %v,%v", pts, err)
	}
}

func TestTrendByModule(t *testing.T) {
	addr := startTCP(t)
	dir := t.TempDir()
	cfg := dir + "/checkfleet.yml"
	if err := os.WriteFile(cfg,
		[]byte("timeout_seconds: 5\nchecks:\n  tcp:\n    targets:\n      - address: \""+addr+"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := NewApp("test")
	app.RunChecks(cfg, "")
	app.RunChecks(cfg, "")

	mt, err := app.TrendByModule(cfg, 10)
	if err != nil {
		t.Fatalf("TrendByModule: %v", err)
	}
	if len(mt.Modules) != 1 || mt.Modules[0] != "tcp" {
		t.Fatalf("modules = %v, want [tcp]", mt.Modules)
	}
	if len(mt.Runs) != 2 {
		t.Fatalf("want 2 runs, got %d", len(mt.Runs))
	}
	if got := mt.Runs[0].Worst["tcp"]; got != "OK" {
		t.Errorf("run[0] tcp worst = %q, want OK", got)
	}

	// Empty config path is a no-op, not an error.
	if mt, err := app.TrendByModule("", 10); err != nil || len(mt.Modules) != 0 || len(mt.Runs) != 0 {
		t.Errorf("empty path: want empty/no-error, got %+v %v", mt, err)
	}
}

func TestWorseOf(t *testing.T) {
	cases := []struct{ a, b, want string }{
		{"OK", "WARN", "WARN"},
		{"ERROR", "BAD", "ERROR"},
		{"BAD", "OK", "BAD"},
		{"WARN", "WARN", "WARN"},
	}
	for _, c := range cases {
		if got := worseOf(c.a, c.b); got != c.want {
			t.Errorf("worseOf(%q,%q) = %q, want %q", c.a, c.b, got, c.want)
		}
	}
}

func TestEnsureStarterConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)             // darwin: UserConfigDir = $HOME/Library/Application Support
	t.Setenv("XDG_CONFIG_HOME", tmp)  // linux: UserConfigDir = $XDG_CONFIG_HOME

	p, created, err := ensureStarterConfig()
	if err != nil || !created {
		t.Fatalf("first call: created=%v err=%v", created, err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("starter config not written: %v", err)
	}
	cfg, err := engine.LoadBytes(b)
	if err != nil || len(engine.Validate(cfg)) != 0 {
		t.Fatalf("starter config must load and validate: err=%v problems=%v", err, engine.Validate(cfg))
	}
	// Idempotent: a second call finds the file and does not recreate it.
	if _, created2, _ := ensureStarterConfig(); created2 {
		t.Error("second call should not recreate an existing config")
	}
}
