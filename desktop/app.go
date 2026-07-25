package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/moduledoc"
	"github.com/Allan-Nava/checkfleet/internal/output"
	"github.com/Allan-Nava/checkfleet/internal/registry"
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
	return rep
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
}

// StartupConfig lets the app open straight into a fleet: CHECKFLEET_CONFIG picks
// the config (falling back to ./checkfleet.yml) and CHECKFLEET_AUTORUN=1 runs it
// on load. Handy for "open with" and used by the end-to-end test.
func (a *App) StartupConfig() Startup {
	path := os.Getenv("CHECKFLEET_CONFIG")
	if path == "" {
		path = a.DefaultConfigPath()
	}
	return Startup{Path: path, AutoRun: os.Getenv("CHECKFLEET_AUTORUN") == "1"}
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
