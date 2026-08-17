package engine

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func ppConfig() *Config {
	return &Config{
		Maintenance: []MaintenanceWindow{{Check: "redis", Action: "mute"}},
		Runbooks:    []RunbookRule{{Check: "postgres", Runbook: "https://wiki/pg", Remediation: "check lag"}},
	}
}

func ppFindings() []Finding {
	return []Finding{
		{Check: "postgres", Target: "db-01", Status: BAD, Message: "lag"},
		{Check: "redis", Target: "cache-01", Status: BAD, Message: "down"},
		{Check: "http", Target: "web", Status: OK, Message: "200"},
	}
}

func TestPostProcessMutesThenAnnotates(t *testing.T) {
	got := PostProcess(Result{Findings: ppFindings()}, ppConfig(), time.Now())
	if len(got.Findings) != 2 {
		t.Fatalf("got %d findings, want 2 (redis muted): %+v", len(got.Findings), got.Findings)
	}
	for _, f := range got.Findings {
		if f.Check == "redis" {
			t.Error("the maintenance window did not drop the redis finding")
		}
		if f.Check == "postgres" && f.Runbook != "https://wiki/pg" {
			t.Errorf("postgres finding lost its hint: %+v", f)
		}
	}
}

// TestPostProcessOrderMattersForMutedFindings: annotating before muting would
// spend work on rows nobody sees, and could carry a hint for a muted finding
// into a sink that only reads hints.
func TestPostProcessDoesNotAnnotateMutedFindings(t *testing.T) {
	cfg := &Config{
		Maintenance: []MaintenanceWindow{{Check: "postgres", Action: "mute"}},
		Runbooks:    []RunbookRule{{Runbook: "https://wiki/all"}},
	}
	got := PostProcess(Result{Findings: ppFindings()}, cfg, time.Now())
	for _, f := range got.Findings {
		if f.Check == "postgres" {
			t.Error("a muted finding must not survive to be annotated")
		}
	}
}

func TestPostProcessWithNilConfigIsInert(t *testing.T) {
	in := ppFindings()
	got := PostProcess(Result{Findings: in}, nil, time.Now())
	if len(got.Findings) != len(in) {
		t.Errorf("nil config must change nothing, got %d of %d", len(got.Findings), len(in))
	}
}

func TestPostProcessDoesNotMutateInput(t *testing.T) {
	in := ppFindings()
	PostProcess(Result{Findings: in}, ppConfig(), time.Now())
	if in[0].Runbook != "" {
		t.Errorf("input finding was annotated in place: %+v", in[0])
	}
	if len(in) != 3 {
		t.Errorf("input slice was truncated to %d", len(in))
	}
}

func TestPostProcessKeepsResultMetadata(t *testing.T) {
	res := Result{
		Findings: ppFindings(),
		Duration: 1500 * time.Millisecond,
		Labels:   map[string]string{"env": "prod"},
	}
	got := PostProcess(res, ppConfig(), time.Now())
	if got.Duration != res.Duration || got.Labels["env"] != "prod" {
		t.Errorf("PostProcess dropped result metadata: %+v", got)
	}
}

// pipelineSteps are the post-run transformations that must reach every
// interface. Adding one to PostProcess is how you register it.
var pipelineSteps = []string{"ApplyMaintenance", "ApplyDependencies", "ApplyRunbooks"}

// TestPostProcessIsTheOnlyPipeline is the parity gate (CF-163), and the reason
// this file exists.
//
// Before it, the steps were open-coded at four call sites and the desktop had
// silently drifted: it applied ApplyRunbooks but not ApplyMaintenance, so an
// active maintenance window silenced `check` and not the GUI — the same config
// giving two verdicts depending on which interface you opened.
//
// The test walks the CLI and desktop sources and fails if any of them calls a
// pipeline step directly instead of going through PostProcess. A step added in
// one place therefore cannot ship without the others.
func TestPostProcessIsTheOnlyPipeline(t *testing.T) {
	roots := map[string]string{
		"cli":     filepath.Join("..", "..", "cmd", "checkfleet"),
		"desktop": filepath.Join("..", "..", "desktop"),
	}
	checked := 0
	for name, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			path := filepath.Join(root, e.Name())
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, src, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			checked++
			ast.Inspect(file, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				pkg, isIdent := sel.X.(*ast.Ident)
				if !isIdent || pkg.Name != "engine" {
					return true
				}
				for _, step := range pipelineSteps {
					if sel.Sel.Name == step {
						t.Errorf("%s calls engine.%s directly — use engine.PostProcess so every interface gets it", path, step)
					}
				}
				return true
			})
		}
	}
	// Without this the walk could silently examine nothing and always pass.
	if checked < 10 {
		t.Fatalf("only parsed %d source files; the walk is broken, not the code", checked)
	}
}

// TestPipelineStepsAreAllRegistered keeps the list above honest: every step
// PostProcess performs must be named in pipelineSteps, or the gate would not
// notice a call site bypassing it.
func TestPipelineStepsAreAllRegistered(t *testing.T) {
	src, err := os.ReadFile("postprocess.go")
	if err != nil {
		t.Fatal(err)
	}
	file, err := parser.ParseFile(token.NewFileSet(), "postprocess.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	registered := map[string]bool{}
	for _, s := range pipelineSteps {
		registered[s] = true
	}
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		id, ok := call.Fun.(*ast.Ident)
		if !ok || !strings.HasPrefix(id.Name, "Apply") {
			return true
		}
		if !registered[id.Name] {
			t.Errorf("PostProcess calls %s but it is not in pipelineSteps — the parity gate would miss a bypass", id.Name)
		}
		return true
	})
}
