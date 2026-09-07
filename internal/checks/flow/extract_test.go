package flow

import (
	"net/http"
	"strings"
	"testing"
)

func response(headers map[string]string) *http.Response {
	h := http.Header{}
	for k, v := range headers {
		h.Set(k, v)
	}
	return &http.Response{Header: h}
}

func TestJSONSelector(t *testing.T) {
	body := []byte(`{
	  "access_token": "tok-1",
	  "expires_in": 3600,
	  "rate": 1.5,
	  "active": true,
	  "items": [{"id": 7}, {"id": 8}],
	  "nothing": null
	}`)
	for _, tc := range []struct{ path, want string }{
		{"access_token", "tok-1"},
		{"expires_in", "3600"}, // an integer must not come back as "3600.0"
		{"rate", "1.5"},
		{"active", "true"},
		{"items.1.id", "8"},
	} {
		got, err := extract("json:"+tc.path, response(nil), body)
		if err != nil {
			t.Errorf("json:%s: %v", tc.path, err)
			continue
		}
		if got != tc.want {
			t.Errorf("json:%s = %q, want %q", tc.path, got, tc.want)
		}
	}

	for _, tc := range []struct{ path, wantErr string }{
		{"absent", `no "absent" at this level`},
		{"items.9.id", "out of range"},
		{"items.first.id", "not an index"},
		{"access_token.deeper", "has no"},
		{"nothing", "null"},
		{"items", "not a scalar"},
	} {
		_, err := extract("json:"+tc.path, response(nil), body)
		if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("json:%s: want an error containing %q, got %v", tc.path, tc.wantErr, err)
		}
	}
}

func TestJSONSelectorOnANonJSONBody(t *testing.T) {
	_, err := extract("json:token", response(nil), []byte("<html>nope</html>"))
	if err == nil || !strings.Contains(err.Error(), "not JSON") {
		t.Fatalf("want a clear error, got %v", err)
	}
	// The body itself must not travel: it is the target's, not the config's.
	if err != nil && strings.Contains(err.Error(), "nope") {
		t.Errorf("the response body leaked into the error: %v", err)
	}
}

func TestHeaderSelector(t *testing.T) {
	res := response(map[string]string{"X-Request-Id": "req-42"})
	got, err := extract("header:X-Request-Id", res, nil)
	if err != nil || got != "req-42" {
		t.Fatalf("got %q, %v", got, err)
	}
	// Header lookup is case-insensitive, as HTTP is.
	if got, err := extract("header:x-request-id", res, nil); err != nil || got != "req-42" {
		t.Errorf("case-insensitive lookup failed: %q, %v", got, err)
	}
	if _, err := extract("header:X-Absent", res, nil); err == nil {
		t.Error("an absent header must be an error, not an empty capture")
	}
}

func TestRegexSelector(t *testing.T) {
	body := []byte(`<input name="csrf" value="abc123">`)
	got, err := extract(`regex:value="([^"]+)"`, response(nil), body)
	if err != nil || got != "abc123" {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := extract(`regex:value="[^"]+"`, response(nil), body); err == nil ||
		!strings.Contains(err.Error(), "capture group") {
		t.Errorf("a pattern with no capture group must say so, got %v", err)
	}
	if _, err := extract(`regex:nomatch=(x)`, response(nil), body); err == nil {
		t.Error("no match must be an error")
	}
	if _, err := extract(`regex:([`, response(nil), body); err == nil ||
		!strings.Contains(err.Error(), "invalid pattern") {
		t.Errorf("an invalid pattern must say so, got %v", err)
	}
}

func TestUnknownSelector(t *testing.T) {
	for _, sel := range []string{"xpath://a", "json", "", "jq:.token"} {
		if _, err := extract(sel, response(nil), []byte(`{}`)); err == nil {
			t.Errorf("selector %q should be rejected", sel)
		}
	}
}

func TestSubstitute(t *testing.T) {
	captured := map[string]string{"token": "tok-1", "id": "7"}
	got, err := substitute("https://api/{{id}}?t={{token}}", captured)
	if err != nil || got != "https://api/7?t=tok-1" {
		t.Fatalf("got %q, %v", got, err)
	}
	if got, err := substitute("no references here", captured); err != nil || got != "no references here" {
		t.Errorf("a plain string must pass through unchanged: %q, %v", got, err)
	}
	_, err = substitute("{{a}} and {{b}}", captured)
	if err == nil || !strings.Contains(err.Error(), `"a"`) || !strings.Contains(err.Error(), `"b"`) {
		t.Errorf("every missing name should be listed, got %v", err)
	}
}

// The substitution is one form of reference and nothing else. This pins that:
// anything that looks like an expression is left alone as literal text rather
// than evaluated, because evaluating config would be a new security surface on
// a process holding production credentials.
func TestSubstitutionIsNotAnExpressionLanguage(t *testing.T) {
	captured := map[string]string{"token": "tok-1"}
	for _, in := range []string{
		"{{ token }}",      // spaces are not part of the reference syntax
		"{{token.length}}", // no property access on a captured value
		"{{token|upper}}",  // no filters
		"${token}",         // not the syntax
		"{{env.HOME}}",     // no ambient namespace to reach into
		"{{}}",             // nothing to resolve
	} {
		got, err := substitute(in, captured)
		if err != nil {
			continue // rejected as an unknown reference, which is also correct
		}
		if strings.Contains(got, "tok-1") {
			t.Errorf("%q resolved to something containing the captured value: %q", in, got)
		}
	}
}
