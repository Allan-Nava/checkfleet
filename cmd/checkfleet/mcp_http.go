// The HTTP transport of `checkfleet mcp` (CF-192).

package main

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

// mcpTokenEnv names the variable holding the bearer token for the HTTP
// transport. Env-only on purpose: a token in a flag lands in shell history and
// in `ps`, a token in the config lands in git.
const mcpTokenEnv = "CHECKFLEET_MCP_TOKEN"

// mcpMaxBody caps a request body. A JSON-RPC call to this server is a few
// hundred bytes; anything near the cap is a mistake or an attack.
const mcpMaxBody = 1 << 20

// serveMCPHTTP exposes the same tool set on POST /mcp, following the MCP
// "Streamable HTTP" transport in its stateless form: every POST carries one
// JSON-RPC message and gets its response as application/json. There are no
// server-initiated messages, so GET (the optional SSE stream) answers 405 and
// no session id is issued — each request stands alone, like a CLI run.
func serveMCPHTTP(env *mcpEnvironment, listen, token string, allowOrigins map[string]bool) error {
	if token == "" {
		return fmt.Errorf("mcp --listen requires %s: the endpoint runs checks against your fleet and is never served unauthenticated", mcpTokenEnv)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	srv := &http.Server{
		Addr:              listen,
		Handler:           mcpHTTPHandler(env, token, allowOrigins),
		ReadHeaderTimeout: 10 * time.Second,
		// No WriteTimeout: a checkfleet_run lasts as long as the slowest module
		// (bounded by timeout_seconds in the config), not by the transport.
	}
	logger.Info("mcp http start", "listen", listen, "endpoint", "/mcp")
	return srv.ListenAndServe()
}

func mcpHTTPHandler(env *mcpEnvironment, token string, allowOrigins map[string]bool) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		// Origin first: the MCP spec requires it against DNS rebinding, where
		// a web page in the operator's browser targets a local listener.
		if o := r.Header.Get("Origin"); o != "" && !allowOrigins[o] {
			http.Error(w, "origin not allowed", http.StatusForbidden)
			return
		}
		if !mcpAuthorized(r, token) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="checkfleet-mcp"`)
			http.Error(w, "missing or invalid bearer token", http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "only POST is supported (stateless transport, no SSE stream)", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, mcpMaxBody+1))
		if err != nil {
			http.Error(w, "cannot read body", http.StatusBadRequest)
			return
		}
		if len(body) > mcpMaxBody {
			http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
			return
		}
		resp := mcpDispatch(body, env)
		if resp == nil {
			// A notification or a response from the client: accepted, nothing
			// to answer.
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	return mux
}

// mcpAuthorized compares the bearer token in constant time.
func mcpAuthorized(r *http.Request, token string) bool {
	got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1
}
