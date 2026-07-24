package main

import "testing"

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

func TestLastPathInt(t *testing.T) {
	if n := lastPathInt("https://github.com/Allan-Nava/checkfleet/issues/42\n"); n != 42 {
		t.Errorf("issue URL: want 42, got %d", n)
	}
	if n := lastPathInt("no number"); n != 0 {
		t.Errorf("string without a number: want 0, got %d", n)
	}
}
