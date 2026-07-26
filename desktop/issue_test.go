package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// OpenIssue must POST to the right GitHub endpoint with bearer auth and return
// the created issue's html_url — driven against a local httptest server so no
// real network or token is involved.
func TestOpenIssueGitHub(t *testing.T) {
	var gotAuth, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"html_url":"https://github.com/acme/fleet/issues/7"}`))
	}))
	defer srv.Close()

	t.Setenv("GITHUB_API", srv.URL)
	t.Setenv("GITHUB_TOKEN", "tkn")
	t.Setenv("GITHUB_REPO", "acme/fleet")

	app := NewApp("test")
	res := app.OpenIssue("github", "certs", "example.com:443", "BAD", "expires in 3 days")

	if !res.OK {
		t.Fatalf("expected ok, got: %s", res.Message)
	}
	if res.URL != "https://github.com/acme/fleet/issues/7" {
		t.Fatalf("url = %q", res.URL)
	}
	if gotPath != "/repos/acme/fleet/issues" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAuth != "Bearer tkn" {
		t.Fatalf("auth = %q, want Bearer tkn", gotAuth)
	}
	var payload map[string]string
	if err := json.Unmarshal([]byte(gotBody), &payload); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if payload["title"] == "" || payload["body"] == "" {
		t.Fatalf("payload missing title/body: %v", payload)
	}
}

// GitLab differs in auth header (PRIVATE-TOKEN), endpoint and the URL field.
func TestOpenIssueGitLab(t *testing.T) {
	var gotTok, gotPath string
	var payload map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTok = r.Header.Get("PRIVATE-TOKEN")
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&payload)
		_, _ = w.Write([]byte(`{"web_url":"https://gitlab.com/acme/fleet/-/issues/3"}`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_API", srv.URL)
	t.Setenv("GITLAB_TOKEN", "glpat")
	t.Setenv("GITLAB_PROJECT", "42")

	app := NewApp("test")
	res := app.OpenIssue("gitlab", "http", "svc:8080", "ERROR", "connection refused")

	if !res.OK || res.URL != "https://gitlab.com/acme/fleet/-/issues/3" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if gotTok != "glpat" {
		t.Fatalf("private-token = %q", gotTok)
	}
	if gotPath != "/projects/42/issues" {
		t.Fatalf("path = %q", gotPath)
	}
	if payload["description"] == "" {
		t.Fatalf("gitlab payload should carry description, got %v", payload)
	}
}

// Without the env vars, OpenIssue must refuse clearly and never hit the network.
func TestOpenIssueNotConfigured(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GITHUB_REPO", "")
	app := NewApp("test")

	res := app.OpenIssue("github", "certs", "t", "BAD", "m")
	if res.OK {
		t.Fatal("must not succeed without token/repo")
	}
	if res.Message == "" {
		t.Fatal("expected a helpful not-configured message")
	}

	if res := app.OpenIssue("bitbucket", "c", "t", "BAD", "m"); res.OK || res.Message == "" {
		t.Fatalf("unknown forge should fail cleanly: %+v", res)
	}
}

// IssueForges reflects which forges have their env set.
func TestIssueForges(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "x")
	t.Setenv("GITHUB_REPO", "a/b")
	t.Setenv("GITLAB_TOKEN", "")
	t.Setenv("GITLAB_PROJECT", "")

	app := NewApp("test")
	f := app.IssueForges()
	if !f["github"] {
		t.Fatal("github should be configured")
	}
	if f["gitlab"] {
		t.Fatal("gitlab should be unconfigured")
	}
}
