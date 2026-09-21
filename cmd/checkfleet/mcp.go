package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
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

func runMCP(args []string) error {
	fs := flag.NewFlagSet("mcp", flag.ExitOnError)
	configPath := fs.String("config", "checkfleet.yml", "YAML config file")
	stack := fs.String("stack", "", "comma-separated stack profiles overlaid in order (last wins): checkfleet.<stack>.yml onto the base")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return serveMCP(&mcpEnvironment{configPath: *configPath, stack: *stack})
}

func serveMCP(env *mcpEnvironment) error {
	reader := bufio.NewReader(os.Stdin)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var req mcpRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			return err
		}
		if req.Method == "" {
			continue
		}
		result, err := handleMCPRequest(req, env)
		if req.ID == nil {
			continue
		}
		if err != nil {
			payload := map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": -32603, "message": err.Error()}}
			if err := json.NewEncoder(os.Stdout).Encode(payload); err != nil {
				return err
			}
			continue
		}
		payload := map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result}
		if err := json.NewEncoder(os.Stdout).Encode(payload); err != nil {
			return err
		}
	}
}

func handleMCPRequest(req mcpRequest, env *mcpEnvironment) (map[string]any, error) {
	switch req.Method {
	case "initialize":
		return map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]any{
				"tools": map[string]any{"listChanged": false},
			},
			"serverInfo": map[string]any{"name": "checkfleet", "version": version},
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": []map[string]any{
			mcpToolDescriptor("checkfleet_run", "Run a configured check module against the selected config.", map[string]any{
				"type": "object",
				"properties": map[string]any{
					"module": map[string]any{"type": "string", "description": "Module name or all"},
				},
				"required": []string{},
			}),
			mcpToolDescriptor("checkfleet_list_modules", "List the modules present in the configuration.", map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}),
			mcpToolDescriptor("checkfleet_validate", "Validate the currently configured YAML without running the checks.", map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}),
		}}, nil
	case "tools/call":
		name, _ := req.Params["name"].(string)
		args := map[string]any{}
		if raw, ok := req.Params["arguments"]; ok {
			if m, ok := raw.(map[string]any); ok {
				args = m
			}
		}
		switch name {
		case "checkfleet_run":
			res, err := runMCPTool(env, args)
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"content":           []map[string]any{{"type": "text", "text": fmt.Sprintf("checkfleet run: %s", prettyMCP(res))}},
				"structuredContent": res,
			}, nil
		case "checkfleet_list_modules":
			res, err := listMCPModules(env)
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"content":           []map[string]any{{"type": "text", "text": fmt.Sprintf("checkfleet modules: %s", prettyMCP(res))}},
				"structuredContent": res,
			}, nil
		case "checkfleet_validate":
			res, err := validateMCPConfig(env)
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"content":           []map[string]any{{"type": "text", "text": fmt.Sprintf("checkfleet validate: %s", prettyMCP(res))}},
				"structuredContent": res,
			}, nil
		default:
			return nil, fmt.Errorf("unknown tool %q", name)
		}
	default:
		return nil, fmt.Errorf("unsupported method %q", req.Method)
	}
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

func validateMCPConfig(env *mcpEnvironment) (map[string]any, error) {
	cfg, err := loadConfig(env.configPath, env.stack)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"ok":      true,
		"modules": registry.Names(cfg),
		"labels":  cfg.Labels,
	}, nil
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
