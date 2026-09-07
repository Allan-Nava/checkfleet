package mediamtx

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// fakeMTX serves /v3/paths/list the way mediamtx does, from the paths given.
func fakeMTX(t *testing.T, items []map[string]any) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v3/paths/list", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"itemCount": len(items), "pageCount": 1, "items": items,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// readyPath is a healthy path: a publisher connected and bytes are moving.
func readyPath(name string, readers int, bytes int64) map[string]any {
	rs := make([]map[string]any, readers)
	for i := range rs {
		rs[i] = map[string]any{"type": "hlsMuxer", "id": fmt.Sprintf("r%d", i)}
	}
	return map[string]any{
		"name": name, "ready": true,
		"source":        map[string]any{"type": "rtmpConn", "id": "pub-1"},
		"tracks":        []string{"H264", "MPEG4-audio"},
		"bytesReceived": bytes, "bytesSent": bytes / 2,
		"readers": rs,
	}
}

func run(t *testing.T, target engine.MediaMTXTarget) []engine.Finding {
	t.Helper()
	return New(engine.MediaMTXConfig{Targets: []engine.MediaMTXTarget{target}}).Run(context.Background())
}

func find(t *testing.T, findings []engine.Finding, target string) engine.Finding {
	t.Helper()
	for _, f := range findings {
		if f.Target == target {
			return f
		}
	}
	t.Fatalf("no finding for %q in %+v", target, findings)
	return engine.Finding{}
}

func TestHealthyPathsAreOK(t *testing.T) {
	srv := fakeMTX(t, []map[string]any{
		readyPath("live/stream1", 3, 1_000_000),
		readyPath("live/stream2", 1, 500_000),
	})
	got := run(t, engine.MediaMTXTarget{Name: "mtx-01", URL: srv.URL})

	api := find(t, got, "mtx-01")
	if api.Status != engine.OK {
		t.Fatalf("want OK from a reachable API, got %s: %s", api.Status, api.Message)
	}
	if api.Value == nil || *api.Value != 2 {
		t.Errorf("the path count should be the metric, got %v", api.Value)
	}

	p := find(t, got, "mtx-01/live/stream1")
	if p.Status != engine.OK {
		t.Fatalf("want OK, got %s: %s", p.Status, p.Message)
	}
	for _, want := range []string{"rtmpConn", "H264+MPEG4-audio", "3 reader(s)"} {
		if !strings.Contains(p.Message, want) {
			t.Errorf("the message should carry %q, got %q", want, p.Message)
		}
	}
	if p.Value == nil || *p.Value != 3 {
		t.Errorf("the reader count should be the metric, got %v", p.Value)
	}
}

// The outage this module exists for: the encoder died, the path went not-ready,
// and every downstream probe still passes because the manifest is cached.
func TestANotReadyPathIsBAD(t *testing.T) {
	srv := fakeMTX(t, []map[string]any{
		{"name": "live/stream1", "ready": false, "source": nil, "bytesReceived": 0},
	})
	got := find(t, run(t, engine.MediaMTXTarget{Name: "mtx-01", URL: srv.URL}), "mtx-01/live/stream1")
	if got.Status != engine.BAD {
		t.Fatalf("want BAD, got %s: %s", got.Status, got.Message)
	}
	if !strings.Contains(got.Message, "no publisher") {
		t.Errorf("the message should say why, got %q", got.Message)
	}
}

// Ready but nothing has arrived: the publisher connected and went silent, which
// looks identical to health from the outside.
func TestAStalledIngestIsBAD(t *testing.T) {
	srv := fakeMTX(t, []map[string]any{readyPath("live/stream1", 2, 0)})
	got := find(t, run(t, engine.MediaMTXTarget{Name: "mtx-01", URL: srv.URL}), "mtx-01/live/stream1")
	if got.Status != engine.BAD {
		t.Fatalf("want BAD from a ready path receiving nothing, got %s: %s", got.Status, got.Message)
	}
	if !strings.Contains(got.Message, "stalled") {
		t.Errorf("the message should name the stall, got %q", got.Message)
	}
}

// expect_paths is the difference between "the paths that exist are fine" and
// "the paths that must be on the air are on the air". A path that vanished from
// the config is invisible to the first question.
func TestAnExpectedPathThatDoesNotExistIsBAD(t *testing.T) {
	srv := fakeMTX(t, []map[string]any{readyPath("live/stream1", 1, 1000)})
	got := run(t, engine.MediaMTXTarget{
		Name: "mtx-01", URL: srv.URL,
		ExpectPaths: []string{"live/stream1", "live/gone"},
	})
	if f := find(t, got, "mtx-01/live/gone"); f.Status != engine.BAD ||
		!strings.Contains(f.Message, "does not exist") {
		t.Fatalf("want BAD for the missing path, got %s: %s", f.Status, f.Message)
	}
	if f := find(t, got, "mtx-01/live/stream1"); f.Status != engine.OK {
		t.Errorf("the existing path should stay OK, got %s: %s", f.Status, f.Message)
	}
	// Only the expected paths are judged when expect_paths is set.
	if len(got) != 3 {
		t.Errorf("want the API finding plus one per expected path, got %d: %+v", len(got), got)
	}
}

// A stream with no viewers at 04:00 is not a fault, so this is opt-in.
func TestNoReadersOnlyWarnsWhenAskedTo(t *testing.T) {
	srv := fakeMTX(t, []map[string]any{readyPath("live/stream1", 0, 1000)})

	quiet := find(t, run(t, engine.MediaMTXTarget{Name: "m", URL: srv.URL}), "m/live/stream1")
	if quiet.Status != engine.OK {
		t.Fatalf("a path with no readers is OK by default, got %s: %s", quiet.Status, quiet.Message)
	}
	loud := find(t, run(t, engine.MediaMTXTarget{Name: "m", URL: srv.URL, WarnNoReaders: true}),
		"m/live/stream1")
	if loud.Status != engine.WARN {
		t.Fatalf("want WARN with warn_no_readers, got %s: %s", loud.Status, loud.Message)
	}
}

// ERROR means the check could not measure. An unreachable or rejecting API is
// not a broken stream.
func TestAPIFailuresAreERROR(t *testing.T) {
	unreachable := run(t, engine.MediaMTXTarget{Name: "down", URL: "http://127.0.0.1:1"})
	if len(unreachable) != 1 || unreachable[0].Status != engine.ERROR {
		t.Fatalf("want a single ERROR, got %+v", unreachable)
	}

	for _, code := range []int{http.StatusUnauthorized, http.StatusInternalServerError} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(code)
		}))
		got := run(t, engine.MediaMTXTarget{Name: "m", URL: srv.URL})
		srv.Close()
		if len(got) != 1 || got[0].Status != engine.ERROR {
			t.Fatalf("HTTP %d should be a single ERROR, got %+v", code, got)
		}
		if !strings.Contains(got[0].Message, fmt.Sprint(code)) {
			t.Errorf("HTTP %d: the message should name the status, got %q", code, got[0].Message)
		}
	}
}

func TestCredentialsComeFromTheEnvironmentAndDoNotLeak(t *testing.T) {
	var gotUser, gotPass string
	var ok bool
	mux := http.NewServeMux()
	mux.HandleFunc("/v3/paths/list", func(w http.ResponseWriter, r *http.Request) {
		gotUser, gotPass, ok = r.BasicAuth()
		_ = json.NewEncoder(w).Encode(map[string]any{"itemCount": 0, "pageCount": 1, "items": []any{}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	t.Setenv("CF_TEST_MTX_PASS", "s3cr3t")
	got := run(t, engine.MediaMTXTarget{
		Name: "m", URL: srv.URL, Username: "admin", PasswordEnv: "CF_TEST_MTX_PASS",
	})
	if !ok || gotUser != "admin" || gotPass != "s3cr3t" {
		t.Fatalf("basic auth did not reach the API: %q/%q ok=%v", gotUser, gotPass, ok)
	}
	for _, f := range got {
		if strings.Contains(f.Message, "s3cr3t") {
			t.Errorf("the password leaked into a finding: %q", f.Message)
		}
	}
}

// A server with more paths than fit one page must not report a truncated fleet:
// silently missing paths is the same failure as not checking them.
func TestPaginationIsFollowed(t *testing.T) {
	pages := [][]map[string]any{
		{readyPath("live/a", 1, 100)},
		{readyPath("live/b", 1, 100)},
	}
	var served []string
	mux := http.NewServeMux()
	mux.HandleFunc("/v3/paths/list", func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		served = append(served, page)
		i := 0
		if page == "1" {
			i = 1
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"itemCount": 2, "pageCount": len(pages), "items": pages[i],
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	got := run(t, engine.MediaMTXTarget{Name: "m", URL: srv.URL})
	if len(served) != 2 {
		t.Fatalf("both pages should be fetched, got %v", served)
	}
	find(t, got, "m/live/a")
	find(t, got, "m/live/b")
	if api := find(t, got, "m"); api.Value == nil || *api.Value != 2 {
		t.Errorf("both pages should be counted, got %v", api.Value)
	}
}

func TestAnIdleServerIsNotAFault(t *testing.T) {
	srv := fakeMTX(t, nil)
	got := run(t, engine.MediaMTXTarget{Name: "m", URL: srv.URL})
	if len(got) != 1 || got[0].Status != engine.OK {
		t.Fatalf("a server with no paths and no expectation is OK, got %+v", got)
	}
}

func TestNameAndEmptyConfig(t *testing.T) {
	c := New(engine.MediaMTXConfig{})
	if c.Name() != "mediamtx" {
		t.Errorf("want mediamtx, got %q", c.Name())
	}
	if got := c.Run(context.Background()); len(got) != 0 {
		t.Errorf("no targets should yield no findings, got %+v", got)
	}
}

func TestAMalformedResponseIsERRORAndDoesNotEchoTheBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"items": "live/secret-path-name"}`))
	}))
	defer srv.Close()

	got := run(t, engine.MediaMTXTarget{Name: "m", URL: srv.URL})
	if len(got) != 1 || got[0].Status != engine.ERROR {
		t.Fatalf("want a single ERROR, got %+v", got)
	}
	if strings.Contains(got[0].Message, "secret-path-name") {
		t.Errorf("the response body leaked into the finding: %q", got[0].Message)
	}
}
