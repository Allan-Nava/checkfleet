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
			t.Errorf("idFromTitle(%q): atteso %q, avuto %q", title, want, got)
		}
	}
}

func TestLastPathInt(t *testing.T) {
	if n := lastPathInt("https://github.com/Allan-Nava/checkfleet/issues/42\n"); n != 42 {
		t.Errorf("URL issue: atteso 42, avuto %d", n)
	}
	if n := lastPathInt("nessun numero"); n != 0 {
		t.Errorf("stringa senza numero: atteso 0, avuto %d", n)
	}
}
