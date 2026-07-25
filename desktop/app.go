package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/history"
	"github.com/Allan-Nava/checkfleet/internal/moduledoc"
	"github.com/Allan-Nava/checkfleet/internal/output"
	"github.com/Allan-Nava/checkfleet/internal/registry"
	"github.com/Allan-Nava/checkfleet/internal/scaffold"
	"github.com/gen2brain/beeep"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App holds the GUI state. The last run is cached so exports don't re-run the
// checks. All check logic lives in internal/* — this is glue only.
type App struct {
	ctx     context.Context
	version string

	mu    sync.Mutex
	last  engine.Result
	title string
	prev  map[string]engine.Status // previous run's statuses, for the diff view
}

// Change is one status transition between the previous run and this one.
type Change struct {
	Check  string `json:"check"`
	Target string `json:"target"`
	From   string `json:"from"`
	To     string `json:"to"`
	Kind   string `json:"kind"` // new | resolved | worsened | improved
}

// NewApp returns an App tagged with the build version.
func NewApp(version string) *App { return &App{version: version} }

// startup captures the Wails context for dialogs and events.
func (a *App) startup(ctx context.Context) { a.ctx = ctx }

// Report is the JSON-friendly view of a run sent to the frontend.
type Report struct {
	ConfigPath string           `json:"configPath"`
	Stack      string           `json:"stack"`
	Modules    []string         `json:"modules"`
	Findings   []engine.Finding `json:"findings"`
	OK         int              `json:"ok"`
	WARN       int              `json:"warn"`
	BAD        int              `json:"bad"`
	ERROR      int              `json:"error"`
	Worst      string           `json:"worst"`
	DurationMs int64            `json:"durationMs"`
	Started    string           `json:"started"`
	Changes    []Change         `json:"changes"`
	Err        string           `json:"err,omitempty"`
}

// diffSep separates check and target in the diff key (a byte that can't appear
// in either), so a target containing "/" is handled correctly.
const diffSep = "\x1f"

// Version returns the build version (shown in the UI footer).
func (a *App) Version() string { return a.version }

// RunChecks loads the config (optionally overlaying a stack) and runs every
// configured module, returning a summarized report. Any load error is returned
// in Report.Err rather than as a Go error, so the UI can render it inline.
func (a *App) RunChecks(configPath, stack string) Report {
	rep := Report{ConfigPath: configPath, Stack: stack}
	if strings.TrimSpace(configPath) == "" {
		rep.Err = "no configuration file selected"
		return rep
	}
	cfg, err := loadConfig(configPath, stack)
	if err != nil {
		rep.Err = err.Error()
		return rep
	}
	checks := registry.Configured(cfg)
	if len(checks) == 0 {
		rep.Err = fmt.Sprintf("no module configured in %s", configPath)
		return rep
	}

	res := engine.RunWith(a.context(), checks, engine.Options{
		Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second,
		Retries: cfg.Retries,
		Backoff: time.Duration(cfg.RetryBackoffMS) * time.Millisecond,
	})

	a.mu.Lock()
	a.last = res
	a.title = "all"
	a.mu.Unlock()

	sum := engine.Summarize(res.Findings)
	rep.Modules = registry.Names(cfg)
	rep.Findings = res.Findings
	rep.OK = sum[engine.OK]
	rep.WARN = sum[engine.WARN]
	rep.BAD = sum[engine.BAD]
	rep.ERROR = sum[engine.ERROR]
	rep.Worst = string(engine.Worst(res.Findings))
	rep.DurationMs = res.Duration.Milliseconds()
	rep.Started = res.Started.Format(time.RFC3339)

	// Diff vs the previous run in this session (skipped on the first run).
	curr := make(map[string]engine.Status, len(res.Findings))
	for _, f := range res.Findings {
		curr[f.Check+diffSep+f.Target] = f.Status
	}
	if a.prev != nil {
		for _, c := range engine.DiffStatus(a.prev, curr) {
			check, target, _ := strings.Cut(c.Key, diffSep)
			rep.Changes = append(rep.Changes, Change{
				Check: check, Target: target,
				From: string(c.From), To: string(c.To), Kind: string(c.Kind),
			})
		}
	}
	a.prev = curr

	// Persist a compact snapshot so the trend survives restarts (best-effort).
	if p := historyPath(configPath); p != "" {
		entries := make([]history.Entry, len(res.Findings))
		for i, f := range res.Findings {
			entries[i] = history.Entry{Check: f.Check, Target: f.Target, Status: string(f.Status)}
		}
		_ = history.Open(p).Append(history.Record{Unix: res.Started.Unix(), Entries: entries})
	}
	return rep
}

// TrendPoint is one past run's rollup, for the persistent trend view.
type TrendPoint struct {
	Unix  int64  `json:"unix"`
	Worst string `json:"worst"`
	OK    int    `json:"ok"`
	WARN  int    `json:"warn"`
	BAD   int    `json:"bad"`
	ERROR int    `json:"error"`
}

// ModuleTrend is the per-module history used by the dashboard heatmap (CF-93):
// a sorted list of modules seen across the recent runs, plus one column per run
// carrying the worst status each module reached in that run.
type ModuleTrend struct {
	Modules []string         `json:"modules"`
	Runs    []ModuleTrendRun `json:"runs"`
}

// ModuleTrendRun is one run's worst status per module (missing = module absent).
type ModuleTrendRun struct {
	Unix  int64             `json:"unix"`
	Worst map[string]string `json:"worst"`
}

var statusRank = map[string]int{"OK": 0, "WARN": 1, "BAD": 2, "ERROR": 3}

// worseOf returns the more severe of two statuses (ERROR>BAD>WARN>OK).
func worseOf(a, b string) string {
	if statusRank[b] > statusRank[a] {
		return b
	}
	return a
}

// TrendByModule returns the last n persisted runs collapsed per module, so the
// GUI can draw a module×run heatmap and drill into a single module's history.
// Same persistence as Trend (survives restarts).
func (a *App) TrendByModule(configPath string, n int) (ModuleTrend, error) {
	p := historyPath(configPath)
	if p == "" {
		return ModuleTrend{}, nil
	}
	records, err := history.Open(p).Recent(n)
	if err != nil {
		return ModuleTrend{}, err
	}
	seen := map[string]bool{}
	runs := make([]ModuleTrendRun, 0, len(records))
	for _, r := range records {
		worst := map[string]string{}
		for _, e := range r.Entries {
			seen[e.Check] = true
			if cur, ok := worst[e.Check]; ok {
				worst[e.Check] = worseOf(cur, e.Status)
			} else {
				worst[e.Check] = e.Status
			}
		}
		runs = append(runs, ModuleTrendRun{Unix: r.Unix, Worst: worst})
	}
	modules := make([]string, 0, len(seen))
	for m := range seen {
		modules = append(modules, m)
	}
	sort.Strings(modules)
	return ModuleTrend{Modules: modules, Runs: runs}, nil
}

// Trend returns the last n persisted runs for a config (oldest first), so the
// GUI can draw a worst-status sparkline that survives restarts — unlike the
// in-session diff (CF-64).
func (a *App) Trend(configPath string, n int) ([]TrendPoint, error) {
	p := historyPath(configPath)
	if p == "" {
		return nil, nil
	}
	records, err := history.Open(p).Recent(n)
	if err != nil {
		return nil, err
	}
	points := make([]TrendPoint, 0, len(records))
	for _, r := range records {
		tp := TrendPoint{Unix: r.Unix}
		for _, e := range r.Entries {
			switch e.Status {
			case "OK":
				tp.OK++
			case "WARN":
				tp.WARN++
			case "BAD":
				tp.BAD++
			case "ERROR":
				tp.ERROR++
			}
		}
		tp.Worst = worstStatus(tp)
		points = append(points, tp)
	}
	return points, nil
}

// worstStatus returns the most severe status present (ERROR>BAD>WARN>OK).
func worstStatus(tp TrendPoint) string {
	switch {
	case tp.ERROR > 0:
		return "ERROR"
	case tp.BAD > 0:
		return "BAD"
	case tp.WARN > 0:
		return "WARN"
	default:
		return "OK"
	}
}

// historyPath returns the per-config history file (a hidden JSONL beside the
// config), or "" when there is no config path.
func historyPath(configPath string) string {
	if strings.TrimSpace(configPath) == "" {
		return ""
	}
	dir := filepath.Dir(configPath)
	base := strings.TrimSuffix(filepath.Base(configPath), filepath.Ext(configPath))
	return filepath.Join(dir, "."+base+".history.jsonl")
}

// ListStacks returns the stack names discovered next to the config file, i.e.
// every checkfleet.<stack>.yml sitting beside checkfleet.yml.
func (a *App) ListStacks(configPath string) []string {
	if strings.TrimSpace(configPath) == "" {
		return nil
	}
	dir := filepath.Dir(configPath)
	base := strings.TrimSuffix(filepath.Base(configPath), filepath.Ext(configPath)) // "checkfleet"
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var stacks []string
	prefix := base + "."
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := filepath.Ext(name)
		if ext != ".yml" && ext != ".yaml" {
			continue
		}
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		// Require the shape checkfleet.<stack>.<ext>: the part after the prefix
		// must still carry the extension, otherwise this is the base file
		// itself (checkfleet.yml → "yml" is the extension, not a stack).
		afterPrefix := strings.TrimPrefix(name, prefix)
		if !strings.HasSuffix(afterPrefix, ext) {
			continue
		}
		if mid := strings.TrimSuffix(afterPrefix, ext); mid != "" {
			stacks = append(stacks, mid)
		}
	}
	sort.Strings(stacks)
	return stacks
}

// DefaultConfigPath returns ./checkfleet.yml when it exists, else "".
func (a *App) DefaultConfigPath() string {
	if wd, err := os.Getwd(); err == nil {
		p := filepath.Join(wd, "checkfleet.yml")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// Startup carries the config the app should open with and whether to run it
// immediately, chosen at launch via the environment.
type Startup struct {
	Path    string `json:"path"`
	AutoRun bool   `json:"autoRun"`
	Created bool   `json:"created"` // a starter config was created on this launch
	Note    string `json:"note"`    // human note (e.g. "created a starter config")
}

// StartupConfig lets the app open straight into a fleet: CHECKFLEET_CONFIG picks
// the config (falling back to ./checkfleet.yml) and CHECKFLEET_AUTORUN=1 runs it
// on load. Handy for "open with" and used by the end-to-end test.
func (a *App) StartupConfig() Startup {
	path := os.Getenv("CHECKFLEET_CONFIG")
	if path == "" {
		path = a.DefaultConfigPath()
	}
	// Nothing configured anywhere: create a valid starter config so the app opens
	// into something editable instead of an empty screen (which looks broken).
	created := false
	note := ""
	if path == "" {
		if p, didCreate, err := ensureStarterConfig(); err == nil {
			path, created = p, didCreate
			if created {
				note = "Created a starter config at " + p + " — edit it to add your targets."
			}
		}
	}
	return Startup{
		Path:    path,
		AutoRun: os.Getenv("CHECKFLEET_AUTORUN") == "1",
		Created: created,
		Note:    note,
	}
}

// ensureStarterConfig writes a valid starter checkfleet.yml under the user config
// directory when none exists yet, returning its path and whether it was just
// created. It never overwrites an existing file.
func ensureStarterConfig() (string, bool, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", false, err
	}
	dir := filepath.Join(base, "checkfleet")
	path := filepath.Join(dir, "checkfleet.yml")
	if _, err := os.Stat(path); err == nil {
		return path, false, nil // already present
	}
	content, err := scaffold.Config(nil) // default starter modules (certs, http)
	if err != nil {
		return "", false, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false, err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", false, err
	}
	return path, true, nil
}

// Validate returns config problems without running any check (empty = valid).
func (a *App) Validate(configPath, stack string) []string {
	cfg, err := loadConfig(configPath, stack)
	if err != nil {
		return []string{err.Error()}
	}
	return engine.Validate(cfg)
}

// Explain returns what a module checks and its thresholds ("" if unknown).
func (a *App) Explain(module string) string {
	d, _ := moduledoc.Doc(module)
	return d
}

// ReadConfig returns the raw text of the config file (empty string if absent).
func (a *App) ReadConfig(path string) (string, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// SaveConfig writes raw config text to the file (creating parent dirs).
func (a *App) SaveConfig(path, content string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("no config path")
	}
	if dir := filepath.Dir(path); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// ValidateText validates unsaved config text and returns the problems (a parse
// error becomes a single problem). Empty result means the config is usable.
func (a *App) ValidateText(content string) []string {
	cfg, err := engine.LoadBytes([]byte(content))
	if err != nil {
		return []string{err.Error()}
	}
	return engine.Validate(cfg)
}

// AddEndpoint inserts a new endpoint into the editor's config text (see
// engine.AddEndpoint) and returns the updated YAML. The result is not saved —
// the frontend puts it back in the textarea for review before Save.
func (a *App) AddEndpoint(yamlText, kind, value, recordType string, expectStatus int) (string, error) {
	return engine.AddEndpoint(yamlText, engine.EndpointSpec{
		Kind: kind, Value: value, RecordType: recordType, ExpectStatus: expectStatus,
	})
}

// ScheduleSnippet returns copy-paste cron and serve commands that run the given
// config on an interval — the "use it like crontab" hint shown in the editor.
func (a *App) ScheduleSnippet(configPath, interval string) string {
	return scheduleSnippet(configPath, interval)
}

// scheduleSnippet builds the cron line and the serve command for a config path.
func scheduleSnippet(configPath, interval string) string {
	if strings.TrimSpace(configPath) == "" {
		configPath = "checkfleet.yml"
	}
	if strings.TrimSpace(interval) == "" {
		interval = "60s"
	}
	mins := intervalMinutes(interval)
	cron := fmt.Sprintf("*/%d * * * * checkfleet check all --config %s --exit-on-bad", mins, configPath)
	serve := fmt.Sprintf("checkfleet serve --config %s --interval %s --listen :9876", configPath, interval)
	return fmt.Sprintf("# cron — run every %d min:\n%s\n\n# or run continuously as a Prometheus exporter:\n%s",
		mins, cron, serve)
}

// intervalMinutes converts a Go-ish duration string (e.g. "30s", "5m", "1h")
// into whole minutes, at least 1. Anything unparseable falls back to 1.
func intervalMinutes(interval string) int {
	interval = strings.TrimSpace(interval)
	if interval == "" {
		return 1
	}
	unit := interval[len(interval)-1]
	numStr := interval
	mult := 1.0 / 60.0 // bare number → seconds
	switch unit {
	case 's':
		numStr, mult = interval[:len(interval)-1], 1.0/60.0
	case 'm':
		numStr, mult = interval[:len(interval)-1], 1
	case 'h':
		numStr, mult = interval[:len(interval)-1], 60
	}
	n, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 1
	}
	mins := int(n * mult)
	if mins < 1 {
		return 1
	}
	return mins
}

// Notify fires a native OS desktop notification (best-effort). The frontend
// calls it after a run whose worst status is BAD/ERROR when notifications are on.
func (a *App) Notify(title, message string) {
	_ = beeep.Notify(title, message, "")
}

// OpenConfigDialog shows a native file picker and returns the chosen path.
func (a *App) OpenConfigDialog() (string, error) {
	return wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{
		Title: "Select checkfleet.yml",
		Filters: []wruntime.FileFilter{
			{DisplayName: "YAML", Pattern: "*.yml;*.yaml"},
		},
	})
}

// ExportMarkdown renders the last run as an ops-style Markdown report.
func (a *App) ExportMarkdown() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return output.Markdown(a.last, a.title)
}

// ExportJSON renders the last run as JSON (includes the "worst" rollup).
func (a *App) ExportJSON() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return output.JSON(a.last)
}

// SaveReport writes the last run to a file the user picks. format is
// "markdown" or "json". Returns the written path ("" if the user cancelled).
func (a *App) SaveReport(format string) (string, error) {
	a.mu.Lock()
	res, title := a.last, a.title
	a.mu.Unlock()

	content, ext, err := renderReport(res, title, format)
	if err != nil {
		return "", err
	}
	path, err := wruntime.SaveFileDialog(a.ctx, wruntime.SaveDialogOptions{
		Title:           "Save report",
		DefaultFilename: "checkfleet-report." + ext,
	})
	if err != nil || path == "" {
		return "", err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// renderReport renders a run in the given format, returning the content and the
// file extension. Reuses the CLI's internal/output renderers.
func renderReport(res engine.Result, title, format string) (content, ext string, err error) {
	switch format {
	case "json":
		s, e := output.JSON(res)
		return s, "json", e
	case "html":
		return output.HTML(res, title), "html", nil
	case "junit":
		s, e := output.JUnit(res, title)
		return s, "xml", e
	case "prometheus":
		return output.Prometheus(res), "prom", nil
	case "otlp":
		s, e := output.OTLP(res)
		return s, "json", e
	default: // markdown
		return output.Markdown(res, title), "md", nil
	}
}

// context returns the Wails context, or Background when running headless.
func (a *App) context() context.Context {
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}

// loadConfig mirrors the CLI: overlay a stack profile when set.
func loadConfig(path, stack string) (*engine.Config, error) {
	if stack != "" {
		return engine.LoadConfigStack(path, stack)
	}
	return engine.LoadConfig(path)
}
