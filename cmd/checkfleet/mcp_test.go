package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestHandleMCPRequestListModules(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "checkfleet.yml")
	if err := os.WriteFile(cfg, []byte("checks:\n  http:\n    targets:\n      - url: https://example.com/\n        expect_status: 200\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	req := mcpRequest{JSONRPC: "2.0", ID: 1, Method: "tools/list", Params: map[string]any{}}
	res, err := handleMCPRequest(req, &mcpEnvironment{configPath: cfg, stack: ""})
	if err != nil {
		t.Fatalf("handleMCPRequest() error = %v", err)
	}

	payload, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) == 0 || string(payload) == "null" {
		t.Fatal("empty MCP response")
	}
	if got := res["tools"]; got == nil {
		t.Fatal("tools/list response missing tools")
	}
}

func TestHandleMCPRequestRunModule(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "checkfleet.yml")
	if err := os.WriteFile(cfg, []byte("checks:\n  http:\n    targets:\n      - url: https://example.com/\n        expect_status: 200\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      "checkfleet_run",
			"arguments": map[string]any{"module": "http"},
		},
	}
	res, err := handleMCPRequest(req, &mcpEnvironment{configPath: cfg, stack: ""})
	if err != nil {
		t.Fatalf("handleMCPRequest() error = %v", err)
	}
	if got := res["structuredContent"]; got == nil {
		t.Fatal("structuredContent missing for tools/call")
	}
}
