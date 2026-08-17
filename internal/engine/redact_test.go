package engine

import (
	"strings"
	"testing"
)

const secret = "hunter2-SUPER-SECRET"

func TestRedactStripsURLPasswords(t *testing.T) {
	cases := []string{
		"dial mongodb://monitor:" + secret + "@db:27017 failed",
		`Get "https://u:` + secret + `@api.example/health": connection refused`,
		"amqp://guest:" + secret + "@rabbit:5672/",
	}
	for _, in := range cases {
		got := Redact(in)
		if strings.Contains(got, secret) {
			t.Errorf("password survived: %q → %q", in, got)
		}
		if !strings.Contains(got, Redacted) {
			t.Errorf("no marker left behind: %q", got)
		}
	}
}

// TestRedactKeepsTheUsername — "who was I connecting as" is the useful half of
// a connection error, and it is not the secret.
func TestRedactKeepsTheUsername(t *testing.T) {
	got := Redact("dial mongodb://monitor:" + secret + "@db:27017 failed")
	if !strings.Contains(got, "monitor") {
		t.Errorf("the username was thrown away too: %q", got)
	}
	if !strings.Contains(got, "db:27017") {
		t.Errorf("the host was thrown away: %q", got)
	}
}

func TestRedactStripsKeyValueSecrets(t *testing.T) {
	cases := []string{
		"failed to connect to `host=db password=" + secret + " user=u`",
		`{"password": "` + secret + `", "host": "db"}`,
		"token=" + secret + " expired",
		"api_key: " + secret,
		"Authorization: Bearer " + secret + "abcdefgh",
	}
	for _, in := range cases {
		if got := Redact(in); strings.Contains(got, secret) {
			t.Errorf("secret survived: %q → %q", in, got)
		}
	}
}

// TestRedactLeavesOrdinaryErrorsAlone — over-redacting costs a debugging hint,
// so it must not fire on text that carries no credential.
func TestRedactLeavesOrdinaryErrorsAlone(t *testing.T) {
	for _, in := range []string{
		"connection refused",
		`dial tcp 127.0.0.1:5432: connect: connection refused`,
		"context deadline exceeded",
		`Get "https://api.example/health": EOF`,
		"",
	} {
		if got := Redact(in); got != in {
			t.Errorf("changed a clean message: %q → %q", in, got)
		}
	}
}

func TestRedactDSNHandlesTheDriverFormat(t *testing.T) {
	// go-sql-driver's DSN has no scheme, so url.Parse cannot help.
	got := RedactDSN("checkfleet:" + secret + "@tcp(127.0.0.1:3306)/db")
	if strings.Contains(got, secret) {
		t.Errorf("password survived: %q", got)
	}
	if !strings.Contains(got, "checkfleet:") || !strings.Contains(got, "tcp(127.0.0.1:3306)") {
		t.Errorf("user or host was lost: %q", got)
	}
	// A DSN with no password must come back recognisable.
	if got := RedactDSN("checkfleet@tcp(127.0.0.1:3306)/db"); strings.Contains(got, Redacted) {
		t.Errorf("a password-less DSN should not gain a marker: %q", got)
	}
}

func TestRedactIsIdempotent(t *testing.T) {
	in := "dial mongodb://u:" + secret + "@db:27017 failed"
	once := Redact(in)
	if twice := Redact(once); twice != once {
		t.Errorf("redacting twice changed the result:\n once: %q\ntwice: %q", once, twice)
	}
}
