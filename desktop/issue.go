package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Open a tracker issue from a finding (CF-113) — the follow-up deferred from
// CF-106. Zero-dep REST: GitHub and GitLab both take a JSON create-issue POST,
// they just differ in the auth header, the endpoint and the field naming.
//
// The repo/project and the token come ONLY from environment variables, never
// the UI — the same no-secrets rule as Send (CF-106). The CLI's report-issues
// reconciler (gh/glab) is a different tool for a different job (bulk sync via
// the forge CLIs); this is a one-click "open an issue for THIS finding".

// IssueResult is the outcome of an OpenIssue call, surfaced to the GUI.
type IssueResult struct {
	OK      bool   `json:"ok"`
	Forge   string `json:"forge"`
	URL     string `json:"url,omitempty"`
	Message string `json:"message"`
}

// forgeSpec names the env vars a forge reads and its default API base (override
// via the apiEnv var, which also makes the client unit-testable against httptest).
type forgeSpec struct {
	tokenEnv, repoEnv, apiEnv, defaultAPI string
}

var issueForges = map[string]forgeSpec{
	"github": {"GITHUB_TOKEN", "GITHUB_REPO", "GITHUB_API", "https://api.github.com"},
	"gitlab": {"GITLAB_TOKEN", "GITLAB_PROJECT", "GITLAB_API", "https://gitlab.com/api/v4"},
}

// IssueForges reports which forges are fully configured (token + repo/project
// env set), so the GUI can enable the menu without ever revealing the token.
func (a *App) IssueForges() map[string]bool {
	out := make(map[string]bool, len(issueForges))
	for name, spec := range issueForges {
		out[name] = os.Getenv(spec.tokenEnv) != "" && os.Getenv(spec.repoEnv) != ""
	}
	return out
}

// OpenIssue opens a tracker issue for one finding on the given forge
// (github|gitlab). Repo/project and token come only from env — never the UI.
func (a *App) OpenIssue(forge, check, target, status, message string) IssueResult {
	spec, ok := issueForges[forge]
	if !ok {
		return IssueResult{Forge: forge, Message: "unknown forge " + forge}
	}
	token, repo := os.Getenv(spec.tokenEnv), os.Getenv(spec.repoEnv)
	if token == "" || repo == "" {
		return IssueResult{Forge: forge, Message: "not configured: set " + spec.tokenEnv + " and " + spec.repoEnv + " (never entered in the app)"}
	}
	api := os.Getenv(spec.apiEnv)
	if api == "" {
		api = spec.defaultAPI
	}
	title := fmt.Sprintf("[checkfleet] %s/%s — %s", check, target, strings.ToUpper(status))
	url, err := postIssue(a.context(), forge, strings.TrimRight(api, "/"), repo, token, title, issueBody(check, target, status, message))
	if err != nil {
		return IssueResult{Forge: forge, Message: err.Error()}
	}
	return IssueResult{OK: true, Forge: forge, URL: url, Message: "opened issue on " + forge}
}

// OpenURL opens a URL in the user's browser (used to jump to a freshly-opened
// issue). No-op without a Wails context.
func (a *App) OpenURL(url string) {
	if a.ctx != nil {
		wruntime.BrowserOpenURL(a.ctx, url)
	}
}

func issueBody(check, target, status, message string) string {
	var b strings.Builder
	b.WriteString("**" + strings.ToUpper(status) + "** — `" + check + "` on `" + target + "`\n\n")
	if message != "" {
		b.WriteString(message + "\n\n")
	}
	b.WriteString("_Opened from checkfleet desktop._")
	return b.String()
}

// postIssue POSTs a create-issue request and returns the new issue's web URL.
// Auth and JSON shape differ per forge; the 2xx contract is shared.
func postIssue(ctx context.Context, forge, api, repo, token, title, body string) (string, error) {
	var (
		endpoint string
		payload  []byte
		auth     func(*http.Request)
		urlField string
	)
	switch forge {
	case "github":
		endpoint = api + "/repos/" + repo + "/issues"
		payload, _ = json.Marshal(map[string]string{"title": title, "body": body})
		auth = func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer "+token)
			r.Header.Set("Accept", "application/vnd.github+json")
		}
		urlField = "html_url"
	case "gitlab":
		endpoint = api + "/projects/" + repo + "/issues"
		payload, _ = json.Marshal(map[string]string{"title": title, "description": body})
		auth = func(r *http.Request) { r.Header.Set("PRIVATE-TOKEN", token) }
		urlField = "web_url"
	default:
		return "", fmt.Errorf("unknown forge %s", forge)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	auth(req)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("opening the issue: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("the forge responded HTTP %d", resp.StatusCode)
	}
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if u, ok := out[urlField].(string); ok {
		return u, nil
	}
	return "", nil
}
