package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mcpConfig writes a config whose only module probes a local httptest server,
// so no test in this file ever touches the network.
func mcpConfig(t *testing.T, body string) (string, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(srv.Close)
	if body == "" {
		body = "checks:\n  http:\n    targets:\n      - url: " + srv.URL + "/\n        expect_status: 200\n"
	}
	cfg := filepath.Join(t.TempDir(), "checkfleet.yml")
	if err := os.WriteFile(cfg, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return cfg, srv
}

// callTool runs one tools/call and returns the result map.
func callTool(t *testing.T, env *mcpEnvironment, name string, args map[string]any) map[string]any {
	t.Helper()
	req := mcpRequest{JSONRPC: "2.0", ID: 1, Method: "tools/call", Params: map[string]any{"name": name, "arguments": args}}
	res, err := handleMCPRequest(req, env)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return res
}

// structured round-trips structuredContent through JSON, as a client sees it.
func structured(t *testing.T, res map[string]any) map[string]any {
	t.Helper()
	if res["isError"] == true {
		t.Fatalf("tool returned an error: %v", res["content"])
	}
	b, err := json.Marshal(res["structuredContent"])
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestMCPToolsListDeclaresEveryTool(t *testing.T) {
	cfg, _ := mcpConfig(t, "")
	res, err := handleMCPRequest(mcpRequest{JSONRPC: "2.0", ID: 1, Method: "tools/list"}, &mcpEnvironment{configPath: cfg})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, tool := range res["tools"].([]map[string]any) {
		got[tool["name"].(string)] = true
		if tool["inputSchema"] == nil {
			t.Errorf("%v: missing inputSchema", tool["name"])
		}
	}
	for _, want := range []string{"checkfleet_run", "checkfleet_list_modules", "checkfleet_validate", "checkfleet_explain", "checkfleet_targets"} {
		if !got[want] {
			t.Errorf("tools/list is missing %s", want)
		}
	}
}

func TestMCPRunModule(t *testing.T) {
	cfg, _ := mcpConfig(t, "")
	out := structured(t, callTool(t, &mcpEnvironment{configPath: cfg}, "checkfleet_run", map[string]any{"module": "http"}))
	if out["status"] != "OK" || out["count"].(float64) != 1 {
		t.Fatalf("want one OK finding, got %v", out)
	}
}

func TestMCPToolErrorIsAResultNotAProtocolError(t *testing.T) {
	cfg, _ := mcpConfig(t, "")
	res := callTool(t, &mcpEnvironment{configPath: cfg}, "checkfleet_run", map[string]any{"module": "nope"})
	if res["isError"] != true {
		t.Fatalf("an unknown module must come back with isError, got %v", res)
	}
}

func TestMCPUnknownToolAndMethodAreProtocolErrors(t *testing.T) {
	env := &mcpEnvironment{configPath: "missing.yml"}
	resp := mcpDispatch([]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"rm_rf"}}`), env)
	if code := resp["error"].(map[string]any)["code"]; code != rpcInvalidParams {
		t.Errorf("unknown tool: want %d, got %v", rpcInvalidParams, code)
	}
	resp = mcpDispatch([]byte(`{"jsonrpc":"2.0","id":2,"method":"resources/list"}`), env)
	if code := resp["error"].(map[string]any)["code"]; code != rpcMethodNotFound {
		t.Errorf("unknown method: want %d, got %v", rpcMethodNotFound, code)
	}
}

func TestMCPValidateReportsProblemsWithSuggestions(t *testing.T) {
	cfg, _ := mcpConfig(t, "checks:\n  postgress:\n    targets: []\n")
	out := structured(t, callTool(t, &mcpEnvironment{configPath: cfg}, "checkfleet_validate", nil))
	if out["valid"] != false {
		t.Fatalf("a misspelled module must not validate: %v", out)
	}
	if !strings.Contains(prettyMCP(out["problems"]), "postgres") {
		t.Errorf("want a did-you-mean suggestion for postgress, got %v", out["problems"])
	}
}

func TestMCPValidateUnreadableConfigIsAResult(t *testing.T) {
	out := structured(t, callTool(t, &mcpEnvironment{configPath: filepath.Join(t.TempDir(), "absent.yml")}, "checkfleet_validate", nil))
	if out["valid"] != false || out["load_error"] == nil {
		t.Fatalf("want valid=false with load_error, got %v", out)
	}
}

func TestMCPValidateValidConfig(t *testing.T) {
	cfg, _ := mcpConfig(t, "")
	out := structured(t, callTool(t, &mcpEnvironment{configPath: cfg}, "checkfleet_validate", nil))
	if out["valid"] != true {
		t.Fatalf("want valid, got %v", out)
	}
}

func TestMCPExplain(t *testing.T) {
	env := &mcpEnvironment{}
	out := structured(t, callTool(t, env, "checkfleet_explain", map[string]any{"module": "certs"}))
	if !strings.Contains(out["description"].(string), "warn_days") {
		t.Errorf("certs description should name its thresholds: %v", out["description"])
	}
	perm, ok := out["permissions"].(map[string]any)
	if !ok || perm["unauthenticated"] != true {
		t.Errorf("certs needs no credential, got %v", out["permissions"])
	}

	list := structured(t, callTool(t, env, "checkfleet_explain", nil))
	if n := len(list["modules"].([]any)); n != len(moduleNames()) {
		t.Errorf("want %d modules listed, got %d", len(moduleNames()), n)
	}

	if res := callTool(t, env, "checkfleet_explain", map[string]any{"module": "nope"}); res["isError"] != true {
		t.Errorf("unknown module must be isError, got %v", res)
	}
}

func TestMCPTargetsNeverLeaksDSN(t *testing.T) {
	cfg, srv := mcpConfig(t, "")
	body := "checks:\n  http:\n    targets:\n      - url: " + srv.URL + "/\n  postgres:\n    targets:\n      - name: db\n        dsn: postgres://admin:hunter2@db.internal:5432/app\n"
	if err := os.WriteFile(cfg, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	env := &mcpEnvironment{configPath: cfg}
	out := structured(t, callTool(t, env, "checkfleet_targets", map[string]any{"discover": false}))
	if out["count"].(float64) != 2 {
		t.Fatalf("want 2 targets, got %v", out)
	}
	if s := prettyMCP(out); strings.Contains(s, "hunter2") || strings.Contains(s, "admin:") {
		t.Fatalf("targets leaked a credential: %s", s)
	}

	only := structured(t, callTool(t, env, "checkfleet_targets", map[string]any{"module": "postgres", "discover": false}))
	if only["count"].(float64) != 1 {
		t.Errorf("module filter: want 1 target, got %v", only)
	}
}

func TestMCPInitializeNegotiatesVersion(t *testing.T) {
	for requested, want := range map[string]string{"2024-11-05": "2024-11-05", "1999-01-01": mcpProtocolVersions[0], "": mcpProtocolVersions[0]} {
		res, err := handleMCPRequest(mcpRequest{JSONRPC: "2.0", ID: 1, Method: "initialize", Params: map[string]any{"protocolVersion": requested}}, &mcpEnvironment{})
		if err != nil {
			t.Fatal(err)
		}
		if res["protocolVersion"] != want {
			t.Errorf("requested %q: want %q, got %v", requested, want, res["protocolVersion"])
		}
	}
}

func TestMCPStdioSurvivesBadLinesAndSkipsNotifications(t *testing.T) {
	cfg, _ := mcpConfig(t, "")
	in := strings.NewReader("not json\n" +
		`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
		`{"jsonrpc":"2.0","id":7,"method":"ping"}`) // no trailing newline: still served
	var out bytes.Buffer
	if err := serveMCP(&mcpEnvironment{configPath: cfg}, in, &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("want a parse error and a ping reply (no reply to the notification), got %q", out.String())
	}
	if !strings.Contains(lines[0], `"code":-32700`) {
		t.Errorf("first line should be a parse error: %s", lines[0])
	}
	if !strings.Contains(lines[1], `"id":7`) || !strings.Contains(lines[1], `"result"`) {
		t.Errorf("second line should answer the ping: %s", lines[1])
	}
}

func TestMCPHTTPRequiresToken(t *testing.T) {
	if err := serveMCPHTTP(&mcpEnvironment{}, "127.0.0.1:0", "", nil); err == nil || !strings.Contains(err.Error(), mcpTokenEnv) {
		t.Fatalf("serving without a token must be refused, got %v", err)
	}
}

func TestMCPHTTPTransport(t *testing.T) {
	cfg, _ := mcpConfig(t, "")
	h := mcpHTTPHandler(&mcpEnvironment{configPath: cfg}, "s3cret", map[string]bool{"https://ok.example": true})
	srv := httptest.NewServer(h)
	defer srv.Close()

	do := func(method, token, origin, body string) *http.Response {
		t.Helper()
		req, err := http.NewRequest(method, srv.URL+"/mcp", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = resp.Body.Close() })
		return resp
	}
	ping := `{"jsonrpc":"2.0","id":1,"method":"ping"}`

	for name, tc := range map[string]struct {
		method, token, origin, body string
		want                        int
	}{
		"no token":       {http.MethodPost, "", "", ping, http.StatusUnauthorized},
		"wrong token":    {http.MethodPost, "nope", "", ping, http.StatusUnauthorized},
		"foreign origin": {http.MethodPost, "s3cret", "https://evil.example", ping, http.StatusForbidden},
		"allowed origin": {http.MethodPost, "s3cret", "https://ok.example", ping, http.StatusOK},
		"get has no sse": {http.MethodGet, "s3cret", "", "", http.StatusMethodNotAllowed},
		"notification":   {http.MethodPost, "s3cret", "", `{"jsonrpc":"2.0","method":"notifications/initialized"}`, http.StatusAccepted},
		"oversized body": {http.MethodPost, "s3cret", "", strings.Repeat(" ", mcpMaxBody+10), http.StatusRequestEntityTooLarge},
		"plain call, ok": {http.MethodPost, "s3cret", "", ping, http.StatusOK},
	} {
		if got := do(tc.method, tc.token, tc.origin, tc.body).StatusCode; got != tc.want {
			t.Errorf("%s: want %d, got %d", name, tc.want, got)
		}
	}

	resp := do(http.MethodPost, "s3cret", "", `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"checkfleet_run","arguments":{"module":"http"}}}`)
	var env struct {
		Result map[string]any `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	sc, _ := env.Result["structuredContent"].(map[string]any)
	if sc["status"] != "OK" {
		t.Fatalf("run over HTTP: want OK, got %v", env.Result)
	}
}
