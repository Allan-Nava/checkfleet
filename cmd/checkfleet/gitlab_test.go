package main

import "testing"

func TestIssueClientFactory(t *testing.T) {
	for _, forge := range []string{"github", "gitlab"} {
		c, ensure, err := issueClient(forge, "checkfleet-finding")
		if err != nil || c == nil || ensure == nil {
			t.Errorf("%s: want a client, got c=%v err=%v", forge, c, err)
		}
	}
	if _, _, err := issueClient("bitbucket", "x"); err == nil {
		t.Error("unknown forge should error")
	}
}
