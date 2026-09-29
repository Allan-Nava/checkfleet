package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/coverage"
	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/moduledoc"
	"github.com/Allan-Nava/checkfleet/internal/registry"
)

type mcpRequest struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      any            `json:"id,omitempty"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params,omitempty"`
}

type mcpEnvironment struct {
	configPath string
	stack      string
}

// JSON-RPC 2.0 error codes used by the server.
const (
	rpcParseError     = -32700
	rpcInvalidRequest = -32600
	rpcMethodNotFound = -32601
	rpcInvalidParams  = -32602
	rpcInternalError  = -32603
)

// rpcError carries a JSON-RPC error code through handleMCPRequest, so the
// transports can tell "unknown method" from "the tool broke".
type rpcError struct {
	code int
	msg  string
}

func (e *rpcError) Error() string { return e.msg }

// mcpProtocolVersions are the MCP revisions this server speaks, newest first.
// The tool surface is identical in all of them; only the transport rules differ.
var mcpProtocolVersions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

func runMCP(args []string) error {
	fs := flag.NewFlagSet("mcp", flag.ExitOnError)
	configPath := fs.String("config", "checkfleet.yml", "YAML config file")
	stack := fs.String("stack", "", "comma-separated stack profiles overlaid in order (last wins): checkfleet.<stack>.yml onto the base")
	listen := fs.String("listen", "", "serve MCP over HTTP on this address (e.g. 127.0.0.1:8765) instead of stdio; requires "+mcpTokenEnv)
	allowOrigin := fs.String("allow-origin", "", "with --listen: comma-separated browser origins allowed to call the endpoint (requests without an Origin header are always allowed)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	env := &mcpEnvironment{configPath: *configPath, stack: *stack}
	if *listen != "" {
		return serveMCPHTTP(env, *listen, os.Getenv(mcpTokenEnv), commaSet(*allowOrigin))
	}
	return serveMCP(env, os.Stdin, os.Stdout)
}

// serveMCP runs the stdio transport: one JSON-RPC message per line in, one
// response per line out. A malformed line is answered with a parse error
// instead of killing the session — a client bug must not take the server down.
func serveMCP(env *mcpEnvironment, in io.Reader, out io.Writer) error {
	reader := bufio.NewReader(in)
	enc := json.NewEncoder(out)
	for {
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			if resp := mcpDispatch([]byte(trimmed), env); resp != nil {
				if werr := enc.Encode(resp); werr != nil {
					return werr
				}
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
	}
}

// mcpDispatch decodes one JSON-RPC message and returns the response envelope,
// or nil for a notification (a message without an id gets no reply).
func mcpDispatch(raw []byte, env *mcpEnvironment) map[string]any {
	var req mcpRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return rpcErrorEnvelope(nil, rpcParseError, "parse error: "+err.Error())
	}
	if req.Method == "" {
		if req.ID == nil {
			return nil
		}
		return rpcErrorEnvelope(req.ID, rpcInvalidRequest, "missing method")
	}
	result, err := handleMCPRequest(req, env)
	if req.ID == nil {
		return nil
	}
	if err != nil {
		code := rpcInternalError
		var re *rpcError
		if errors.As(err, &re) {
			code = re.code
		}
		return rpcErrorEnvelope(req.ID, code, err.Error())
	}
	return map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result}
}

func rpcErrorEnvelope(id any, code int, msg string) map[string]any {
	return map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": msg}}
}

// mcpTools declares the tool set. Every tool is a thin wrapper over the same
// internal packages the CLI uses: nothing here decides what a finding means.
func mcpTools() []map[string]any {
	noArgs := map[string]any{"type": "object", "properties": map[string]any{}}
	return []map[string]any{
		mcpToolDescriptor("checkfleet_run", "Run a configured check module (or all) against the selected config and return the findings, worst first.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"module": map[string]any{"type": "string", "description": "Module name, or \"all\" (default)"},
			},
		}),
		mcpToolDescriptor("checkfleet_list_modules", "List every known module and the ones configured in the config.", noArgs),
		mcpToolDescriptor("checkfleet_validate", "Validate the config without running any check: problems with suggested fixes, advisory notes kept separate.", noArgs),
		mcpToolDescriptor("checkfleet_explain", "Explain what a module checks, its thresholds and the access it needs. Without a module, list every module with a one-line summary.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"module": map[string]any{"type": "string", "description": "Module name; omit to list all modules"},
			},
		}),
		mcpToolDescriptor("checkfleet_targets", "List every configured target across modules (hostnames only, never DSNs), including targets from discovery sources.", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"module":   map[string]any{"type": "string", "description": "Only list targets of this module"},
				"discover": map[string]any{"type": "boolean", "description": "Resolve discovery sources (consul_service, dns_srv, ansible_inventory); default true"},
			},
		}),
	}
}

func handleMCPRequest(req mcpRequest, env *mcpEnvironment) (map[string]any, error) {
	switch req.Method {
	case "initialize":
		requested, _ := req.Params["protocolVersion"].(string)
		return map[string]any{
			"protocolVersion": negotiateMCPVersion(requested),
			"capabilities": map[string]any{
				"tools": map[string]any{"listChanged": false},
			},
			"serverInfo": map[string]any{"name": "checkfleet", "version": version},
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": mcpTools()}, nil
	case "tools/call":
		name, _ := req.Params["name"].(string)
		args := map[string]any{}
		if raw, ok := req.Params["arguments"]; ok {
			if m, ok := raw.(map[string]any); ok {
				args = m
			}
		}
		var tool func(*mcpEnvironment, map[string]any) (map[string]any, error)
		switch name {
		case "checkfleet_run":
			tool = runMCPTool
		case "checkfleet_list_modules":
			tool = func(e *mcpEnvironment, _ map[string]any) (map[string]any, error) { return listMCPModules(e) }
		case "checkfleet_validate":
			tool = func(e *mcpEnvironment, _ map[string]any) (map[string]any, error) { return validateMCPConfig(e) }
		case "checkfleet_explain":
			tool = func(_ *mcpEnvironment, a map[string]any) (map[string]any, error) { return explainMCP(a) }
		case "checkfleet_targets":
			tool = targetsMCP
		default:
			return nil, &rpcError{rpcInvalidParams, fmt.Sprintf("unknown tool %q", name)}
		}
		res, err := tool(env, args)
		if err != nil {
			// A tool that fails is a result the model should read, not a
			// protocol error: MCP reports it with isError so the agent can
			// correct the call (wrong module name, unreadable config…).
			return map[string]any{
				"content": []map[string]any{{"type": "text", "text": err.Error()}},
				"isError": true,
			}, nil
		}
		return map[string]any{
			"content":           []map[string]any{{"type": "text", "text": prettyMCP(res)}},
			"structuredContent": res,
		}, nil
	default:
		return nil, &rpcError{rpcMethodNotFound, fmt.Sprintf("unsupported method %q", req.Method)}
	}
}

// negotiateMCPVersion echoes the client's revision when supported, otherwise
// proposes the newest one, as the MCP lifecycle prescribes.
func negotiateMCPVersion(requested string) string {
	for _, v := range mcpProtocolVersions {
		if v == requested {
			return v
		}
	}
	return mcpProtocolVersions[0]
}

func mcpToolDescriptor(name, description string, inputSchema map[string]any) map[string]any {
	return map[string]any{
		"name":        name,
		"description": description,
		"inputSchema": inputSchema,
	}
}

func prettyMCP(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

func runMCPTool(env *mcpEnvironment, args map[string]any) (map[string]any, error) {
	module, _ := args["module"].(string)
	if module == "" {
		module = "all"
	}
	cfg, err := loadConfig(env.configPath, env.stack)
	if err != nil {
		return nil, err
	}
	specs := registry.Modules(cfg)
	selected := make([]engine.Job, 0, len(specs))
	known := module == "all"
	for _, s := range specs {
		if module != "all" && module != s.Name {
			continue
		}
		known = true
		if !s.Configured {
			if module == s.Name {
				return nil, fmt.Errorf("module %q is not configured in %s", s.Name, env.configPath)
			}
			continue
		}
		selected = append(selected, engine.Job{Check: s.Build(), Opts: registry.OptionsFor(cfg, s.Name, runOptions(cfg))})
	}
	if !known {
		return nil, fmt.Errorf("unknown module %q", module)
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("no module selected (nothing configured for %q)", module)
	}
	res := engine.RunJobsLimited(context.Background(), selected, effectiveConcurrency(-1, cfg))
	res = engine.PostProcess(res, cfg, time.Now())
	out := map[string]any{
		"module":   module,
		"status":   string(engine.Worst(res.Findings)),
		"count":    len(res.Findings),
		"findings": findingsToMCP(res.Findings),
	}
	return out, nil
}

func listMCPModules(env *mcpEnvironment) (map[string]any, error) {
	cfg, err := loadConfig(env.configPath, env.stack)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"modules":    registry.All(cfg),
		"configured": registry.Names(cfg),
	}, nil
}

// validateMCPConfig mirrors `checkfleet validate`: engine.Inspect names the
// problems and their fixes, advisory notes (about this machine, not the config)
// do not make the config invalid, and a config that cannot load is still a
// result — the load error is the most useful thing to hand back.
func validateMCPConfig(env *mcpEnvironment) (map[string]any, error) {
	cfg, loadErr := loadConfig(env.configPath, env.stack)
	problems := engine.Inspect(env.configPath, cfg)
	if problems == nil {
		problems = []engine.Problem{}
	}
	out := map[string]any{
		"config":   env.configPath,
		"valid":    loadErr == nil && !engine.Blocking(problems),
		"problems": problems,
	}
	if loadErr != nil {
		out["load_error"] = loadErr.Error()
		return out, nil
	}
	out["modules"] = registry.Names(cfg)
	if len(cfg.Labels) > 0 {
		out["labels"] = cfg.Labels
	}
	return out, nil
}

// explainMCP returns the module contract from internal/moduledoc: the same text
// as `checkfleet explain` plus the access the module needs (`checkfleet perms`).
func explainMCP(args map[string]any) (map[string]any, error) {
	module, _ := args["module"].(string)
	if module == "" {
		list := make([]map[string]any, 0, len(moduleNames()))
		for _, m := range moduleNames() {
			list = append(list, map[string]any{"module": m, "summary": firstSentence(moduledoc.Docs[m])})
		}
		return map[string]any{"modules": list}, nil
	}
	doc, ok := moduledoc.Doc(module)
	if !ok {
		return nil, fmt.Errorf("unknown module %q (call checkfleet_explain without a module to list them)", module)
	}
	out := map[string]any{"module": module, "description": doc}
	if p, ok := moduledoc.Perms(module); ok {
		perm := map[string]any{
			"summary":         p.Summary,
			"unauthenticated": p.Unauthenticated,
		}
		if len(p.Statements) > 0 {
			perm["statements"] = p.Statements
		}
		if p.NotNeeded != "" {
			perm["not_needed"] = p.NotNeeded
		}
		if p.NeedsJudgement {
			perm["needs_judgement"] = true
		}
		out["permissions"] = perm
	}
	return out, nil
}

// targetsMCP mirrors `checkfleet targets --output json`: coverage.Targets never
// returns a DSN or URI value, only the extracted hostname, so the list is safe
// to hand to a model. Discovery failures are reported next to the targets
// rather than failing the call, exactly as the CLI prints them on stderr.
func targetsMCP(env *mcpEnvironment, args map[string]any) (map[string]any, error) {
	cfg, err := loadConfig(env.configPath, env.stack)
	if err != nil {
		return nil, err
	}
	targets := coverage.Targets(cfg)
	out := map[string]any{}
	discover := true
	if v, ok := args["discover"].(bool); ok {
		discover = v
	}
	if discover {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		found, derrs := coverage.Discovered(ctx, cfg)
		targets = append(targets, found...)
		if len(derrs) > 0 {
			warnings := make([]string, 0, len(derrs))
			for _, m := range sortedKeys(derrs) {
				warnings = append(warnings, fmt.Sprintf("%s: discovery failed: %v", m, derrs[m]))
			}
			out["discovery_errors"] = warnings
		}
	}
	if module, _ := args["module"].(string); module != "" {
		kept := []coverage.Target{}
		for _, t := range targets {
			if t.Module == module {
				kept = append(kept, t)
			}
		}
		if len(kept) == 0 {
			return nil, fmt.Errorf("no targets for module %q in %s", module, env.configPath)
		}
		targets = kept
	}
	if targets == nil {
		targets = []coverage.Target{}
	}
	modules := map[string]bool{}
	for _, t := range targets {
		modules[t.Module] = true
	}
	names := make([]string, 0, len(modules))
	for m := range modules {
		names = append(names, m)
	}
	sort.Strings(names)
	out["targets"] = targets
	out["count"] = len(targets)
	out["modules"] = names
	return out, nil
}

func findingsToMCP(findings []engine.Finding) []map[string]any {
	out := make([]map[string]any, 0, len(findings))
	for _, f := range findings {
		m := map[string]any{
			"check":   f.Check,
			"target":  f.Target,
			"status":  string(f.Status),
			"message": f.Message,
		}
		if f.Value != nil {
			m["value"] = *f.Value
		}
		if f.Unit != "" {
			m["unit"] = f.Unit
		}
		if f.Runbook != "" {
			m["runbook"] = f.Runbook
		}
		if f.Remediation != "" {
			m["remediation"] = f.Remediation
		}
		out = append(out, m)
	}
	return out
}
