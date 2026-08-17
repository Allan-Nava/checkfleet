package registry

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// checksDir is the module tree this guard walks.
const checksDir = "../checks"

// writeVerb finds an HTTP method that changes state on the target.
var writeVerb = regexp.MustCompile(`"(POST|PUT|DELETE|PATCH)"|http\.Method(Post|Put|Delete|Patch)`)

// transportPOST lists the files where POST is the *transport*, not a mutation,
// with the reason. Read-only is the promise the whole project rests on, so an
// exception has to be argued rather than assumed — adding a file here is a
// deliberate act that shows up in review.
var transportPOST = map[string]string{
	"etcd/etcd.go": "etcd's gRPC-JSON gateway requires POST even for reads: " +
		"/v3/maintenance/status, /v3/cluster/member/list and /v3/auth/authenticate " +
		"are all POST-only. None of them writes a key.",
	"grpccheck/grpccheck.go": "gRPC itself is POST over HTTP/2. The call is the " +
		"standard Health/Check RPC, which reads a serving status.",
}

// TestModulesIssueNoWriteVerbs is the static half of the read-only guarantee
// (CF-186). Being allowed to point this tool at production rests on it, and
// until now nothing checked it.
func TestModulesIssueNoWriteVerbs(t *testing.T) {
	files := goFilesUnder(t, checksDir)
	// One file per module plus a handful of adapters. The floor catches a walk
	// that stopped finding anything; it is not a count to keep updated.
	if len(files) < 25 {
		t.Fatalf("walked only %d module files; the walk is broken, not the code", len(files))
	}
	for _, path := range files {
		body := readFile(t, path)
		rel := relTo(path, checksDir)
		for _, m := range writeVerb.FindAllString(body, -1) {
			if reason, ok := transportPOST[rel]; ok {
				if reason == "" {
					t.Errorf("%s: exception with no reason", rel)
				}
				continue
			}
			t.Errorf("%s issues %s — checkfleet only reads. If this is transport "+
				"rather than a mutation, add it to transportPOST with the reason.", rel, m)
		}
	}
}

// writeCommand finds a mutating command in the hand-rolled text protocols.
// Redis and memcached speak plain verbs, so a typo away from `INFO` is a verb
// that changes the server.
var writeCommand = regexp.MustCompile(`\b(SET|DEL|DELETE|FLUSHALL|FLUSHDB|RENAME|EXPIRE|INCR|DECR|ADD|REPLACE|APPEND|TOUCH|SHUTDOWN)\b`)

func TestTextProtocolModulesSendNoWriteCommands(t *testing.T) {
	for _, dir := range []string{"redis", "memcached"} {
		for _, path := range goFilesUnder(t, filepath.Join(checksDir, dir)) {
			body := readFile(t, path)
			// Only look at string literals: a Go identifier called `set` is not
			// a Redis command, and flagging it would make the guard noise.
			for _, lit := range stringLiterals(body) {
				if writeCommand.MatchString(strings.ToUpper(lit)) {
					t.Errorf("%s sends %q — these modules read (INFO / stats) and nothing else",
						relTo(path, checksDir), lit)
				}
			}
		}
	}
}

// TestTheReadOnlyGuardBites proves the scan can reject something, so the
// guarantee is enforced rather than merely asserted.
func TestTheReadOnlyGuardBites(t *testing.T) {
	if !writeVerb.MatchString(`req, _ := http.NewRequest("DELETE", url, nil)`) {
		t.Error("a literal DELETE was not detected")
	}
	if !writeVerb.MatchString(`http.MethodPut`) {
		t.Error("http.MethodPut was not detected")
	}
	if writeVerb.MatchString(`http.MethodGet`) {
		t.Error("GET must not be flagged")
	}
	if !writeCommand.MatchString("FLUSHALL") {
		t.Error("FLUSHALL was not detected")
	}
	if writeCommand.MatchString("INFO") {
		t.Error("INFO must not be flagged")
	}
}

// --- helpers ---------------------------------------------------------------

var literalRe = regexp.MustCompile(`"[^"\n]*"`)

func stringLiterals(body string) []string {
	out := literalRe.FindAllString(body, -1)
	for i, s := range out {
		out[i] = strings.Trim(s, `"`)
	}
	return out
}

func goFilesUnder(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		out = append(out, p)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func relTo(path, root string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}
