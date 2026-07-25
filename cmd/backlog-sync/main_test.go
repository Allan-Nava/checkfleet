package main

import (
	"errors"
	"testing"
)

func TestIDFromTitle(t *testing.T) {
	cases := map[string]string{
		"CF-4 — Modulo patroni":         "CF-4",
		"CF-18 — Packaging desktop":     "CF-18",
		"Random issue not from backlog": "",
		"":                              "",
	}
	for title, want := range cases {
		if got := idFromTitle(title); got != want {
			t.Errorf("idFromTitle(%q): want %q, got %q", title, want, got)
		}
	}
}

func TestIsAlreadyExists(t *testing.T) {
	// The real 422 body gh prints when a milestone title is taken.
	dup := errors.New(`gh api -X POST repos/{owner}/{repo}/milestones -f title=M26: exit status 1: ` +
		`gh: Validation Failed (HTTP 422) {"message":"Validation Failed","errors":` +
		`[{"resource":"Milestone","code":"already_exists","field":"title"}],"status":"422"}`)
	if !isAlreadyExists(dup) {
		t.Error("duplicate-milestone 422: want true, got false")
	}
	if isAlreadyExists(errors.New("gh: Not Found (HTTP 404)")) {
		t.Error("unrelated failure: want false, got true")
	}
	if isAlreadyExists(nil) {
		t.Error("nil error: want false, got true")
	}
}

func TestLastPathInt(t *testing.T) {
	if n := lastPathInt("https://github.com/Allan-Nava/checkfleet/issues/42\n"); n != 42 {
		t.Errorf("issue URL: want 42, got %d", n)
	}
	if n := lastPathInt("no number"); n != 0 {
		t.Errorf("string without a number: want 0, got %d", n)
	}
}
