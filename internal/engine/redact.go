package engine

import (
	"net/url"
	"regexp"
	"strings"
)

// Redact removes credentials from text that is about to become a finding
// message (CF-184).
//
// The rule "checks never log credentials" was a convention held by hand. It
// held — the sentinel test in internal/registry shows no module leaks today —
// but a convention only protects the code already written. A module author
// writing fmt.Sprintf("connect to %s failed", dsn) would break it silently, and
// the classic way a password reaches a log is a connection string inside an
// error.
//
// Wrap anything that might carry one:
//
//	f.Message = fmt.Sprintf("connection failed: %v", engine.Redact(err.Error()))
//
// It is deliberately blunt. Over-redacting an error message costs a debugging
// hint; under-redacting costs a credential rotation.
func Redact(s string) string {
	if s == "" {
		return s
	}
	s = redactURLs(s)
	for _, re := range redactPatterns {
		s = re.ReplaceAllString(s, "${1}"+Redacted)
	}
	return s
}

// Redacted is what replaces a credential, chosen to be obvious in a log rather
// than to look like a value.
const Redacted = "[redacted]"

var redactPatterns = []*regexp.Regexp{
	// key=value in libpq-style DSNs: password=secret, sslpassword=secret
	regexp.MustCompile(`(?i)\b((?:ssl)?password\s*=\s*)[^\s&;"']+`),
	// JSON or struct-ish: "password": "secret", pwd:secret
	regexp.MustCompile(`(?i)("?(?:password|passwd|pwd|secret|token|api[_-]?key)"?\s*[:=]\s*"?)[^\s,}"']+`),
	// Bearer / Basic authorization headers echoed into an error
	regexp.MustCompile(`(?i)\b((?:bearer|basic)\s+)[A-Za-z0-9._~+/=-]{8,}`),
}

// urlRe finds anything that looks like a URL with userinfo, in any surrounding
// punctuation an error message might wrap it in.
var urlRe = regexp.MustCompile(`\b[a-zA-Z][a-zA-Z0-9+.-]*://[^\s"'\x60,)]+`)

// redactURLs strips the password from the userinfo of every URL-shaped token,
// keeping the username: "who was I connecting as" is the useful half, and it is
// not the secret.
func redactURLs(s string) string {
	return urlRe.ReplaceAllStringFunc(s, func(raw string) string {
		u, err := url.Parse(raw)
		if err != nil || u.User == nil {
			return raw
		}
		if _, hasPass := u.User.Password(); !hasPass {
			return raw
		}
		u.User = url.UserPassword(u.User.Username(), Redacted)
		// url.String() percent-encodes the brackets; put them back so the value
		// reads as a marker rather than as noise.
		return strings.ReplaceAll(u.String(), "%5Bredacted%5D", Redacted)
	})
}

// RedactDSN is Redact for a driver DSN of the form user:pass@host, which has no
// scheme for url.Parse to work with (go-sql-driver's format).
func RedactDSN(dsn string) string {
	at := strings.LastIndex(dsn, "@")
	if at < 0 {
		return Redact(dsn)
	}
	head := dsn[:at]
	colon := strings.Index(head, ":")
	if colon < 0 {
		return Redact(dsn) // user with no password
	}
	return head[:colon+1] + Redacted + dsn[at:]
}
