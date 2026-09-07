package flow

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// loginServer is the shape CF-180 exists for: a token endpoint, and a resource
// that only answers to that token.
func loginServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "grant_type=client_credentials") {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("X-Request-Id", "req-42")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "tok-abc123",
			"expires_in":   3600,
			"scopes":       []string{"read", "write"},
		})
	})
	mux.HandleFunc("/me", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok-abc123" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"active":true}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func run(t *testing.T, f engine.Flow) engine.Finding {
	t.Helper()
	findings := New(engine.FlowConfig{Flows: []engine.Flow{f}}).Run(context.Background())
	if len(findings) != 1 {
		t.Fatalf("want one finding per flow, got %d", len(findings))
	}
	return findings[0]
}

func TestAFlowCarriesACapturedValueForward(t *testing.T) {
	srv := loginServer(t)
	got := run(t, engine.Flow{
		Name: "login",
		Steps: []engine.FlowStep{
			{
				Name: "get a token", Method: "POST", URL: srv.URL + "/token",
				Body:    "grant_type=client_credentials",
				Extract: map[string]string{"token": "json:access_token"},
			},
			{
				Name: "use the token", URL: srv.URL + "/me",
				Headers:    map[string]string{"Authorization": "Bearer {{token}}"},
				ExpectBody: `"active":true`,
			},
		},
	})
	if got.Status != engine.OK {
		t.Fatalf("want OK, got %s: %s", got.Status, got.Message)
	}
	if got.Target != "login" {
		t.Errorf("the finding must be named after the flow, got %q", got.Target)
	}
	if !strings.Contains(got.Message, "2 step(s) ok") {
		t.Errorf("unexpected message: %q", got.Message)
	}
}

// The whole point of the module: not "the flow is broken" but which step broke.
func TestTheFindingNamesTheFailingStep(t *testing.T) {
	srv := loginServer(t)
	got := run(t, engine.Flow{
		Name: "login",
		Steps: []engine.FlowStep{
			{Name: "get a token", Method: "POST", URL: srv.URL + "/token",
				Body:    "grant_type=client_credentials",
				Extract: map[string]string{"token": "json:access_token"}},
			// The header is missing, so the resource rejects the request.
			{Name: "use the token", URL: srv.URL + "/me"},
		},
	})
	if got.Status != engine.BAD {
		t.Fatalf("want BAD from a rejected request, got %s: %s", got.Status, got.Message)
	}
	for _, want := range []string{"step 2/2", "use the token", "expected status 200, got 401"} {
		if !strings.Contains(got.Message, want) {
			t.Errorf("the message must contain %q, got %q", want, got.Message)
		}
	}
}

// A captured token must never reach a finding, a terminal, a CI log or the JSON
// output. This is the rule the module is built around, so it gets a test that
// looks for the value itself rather than trusting the code paths.
func TestACapturedValueNeverReachesTheOutput(t *testing.T) {
	srv := loginServer(t)
	for _, f := range []engine.Flow{
		// A flow that succeeds.
		{Name: "ok", Steps: []engine.FlowStep{
			{Method: "POST", URL: srv.URL + "/token", Body: "grant_type=client_credentials",
				Extract: map[string]string{"token": "json:access_token"}},
			{URL: srv.URL + "/me", Headers: map[string]string{"Authorization": "Bearer {{token}}"}},
		}},
		// A flow that fails after capturing, which is where a naive
		// implementation echoes the request it just built.
		{Name: "fails after capturing", Steps: []engine.FlowStep{
			{Method: "POST", URL: srv.URL + "/token", Body: "grant_type=client_credentials",
				Extract: map[string]string{"token": "json:access_token"}},
			{URL: srv.URL + "/me", Headers: map[string]string{"Authorization": "Bearer {{token}}"},
				ExpectStatus: 418},
		}},
		// A flow whose captured value is substituted into the URL and then
		// fails to connect: the transport error quotes the URL back.
		{Name: "captured value in the url", Steps: []engine.FlowStep{
			{Method: "POST", URL: srv.URL + "/token", Body: "grant_type=client_credentials",
				Extract: map[string]string{"token": "json:access_token"}},
			{URL: "http://127.0.0.1:1/{{token}}"},
		}},
	} {
		got := run(t, f)
		if strings.Contains(got.Message, "tok-abc123") {
			t.Errorf("flow %q leaked the captured token into the finding: %q", f.Name, got.Message)
		}
	}
}

// The guard above only means something if it can fail, so this proves the token
// really is in play: substituting it is what makes the flow work at all.
func TestTheLeakGuardIsWatchingARealToken(t *testing.T) {
	srv := loginServer(t)
	got := run(t, engine.Flow{Name: "login", Steps: []engine.FlowStep{
		{Method: "POST", URL: srv.URL + "/token", Body: "grant_type=client_credentials",
			Extract: map[string]string{"token": "json:access_token"}},
		// A wrong literal instead of the captured value: the resource rejects it.
		{URL: srv.URL + "/me", Headers: map[string]string{"Authorization": "Bearer wrong"}},
	}})
	if got.Status != engine.BAD {
		t.Fatalf("the second step only passes with the captured token, so this must fail: %s", got.Message)
	}
}

// ERROR means "the check could not measure", BAD means "the target answered and
// answered wrong". Collapsing the two would report a broken network path as a
// broken service.
func TestNetworkFailureIsERRORNotBAD(t *testing.T) {
	got := run(t, engine.Flow{Name: "unreachable", Steps: []engine.FlowStep{
		{Name: "connect", URL: "http://127.0.0.1:1/"},
	}})
	if got.Status != engine.ERROR {
		t.Fatalf("want ERROR from an unreachable host, got %s: %s", got.Status, got.Message)
	}
	if !strings.Contains(got.Message, "step 1/1") {
		t.Errorf("even an ERROR must name the step: %q", got.Message)
	}
}

// A step that fails stops the flow: every later step would fail for the same
// reason, and the message that matters would be buried under theirs.
func TestAFailedStepStopsTheFlow(t *testing.T) {
	var reached int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/second" {
			reached++
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	got := run(t, engine.Flow{Name: "two steps", Steps: []engine.FlowStep{
		{Name: "first", URL: srv.URL + "/first"},
		{Name: "second", URL: srv.URL + "/second"},
	}})
	if reached != 0 {
		t.Errorf("the second step ran %d time(s) after the first failed", reached)
	}
	if !strings.Contains(got.Message, "step 1/2") {
		t.Errorf("want the first step named, got %q", got.Message)
	}
}

func TestExpectBodyFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"active":false}`))
	}))
	defer srv.Close()

	got := run(t, engine.Flow{Name: "status", Steps: []engine.FlowStep{
		{Name: "read", URL: srv.URL, ExpectBody: `"active":true`},
	}})
	if got.Status != engine.BAD {
		t.Fatalf("want BAD, got %s: %s", got.Status, got.Message)
	}
	if !strings.Contains(got.Message, `does not contain "\"active\":true"`) {
		t.Errorf("the expected substring is config and may be echoed: %q", got.Message)
	}
	// The response body is not config and must not be.
	if strings.Contains(got.Message, "false") {
		t.Errorf("the response body leaked into the finding: %q", got.Message)
	}
}

// A session login needs the cookie the first step set to reach the second.
func TestCookiesCarryAcrossSteps(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "s-1", Path: "/"})
	})
	mux.HandleFunc("/private", func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("session"); err != nil || c.Value != "s-1" {
			w.WriteHeader(http.StatusForbidden)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	steps := []engine.FlowStep{{Name: "login", URL: srv.URL + "/login"}, {Name: "private", URL: srv.URL + "/private"}}
	if got := run(t, engine.Flow{Name: "session", Steps: steps, KeepCookies: true}); got.Status != engine.OK {
		t.Fatalf("want OK with a cookie jar, got %s: %s", got.Status, got.Message)
	}
	// Without the jar the same flow must fail, or the test above proves nothing.
	if got := run(t, engine.Flow{Name: "session", Steps: steps}); got.Status == engine.OK {
		t.Error("without keep_cookies the second step should be rejected")
	}
}

func TestFlowLatencyBudget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()

	got := run(t, engine.Flow{Name: "slow", MaxLatencyMS: 1, Steps: []engine.FlowStep{
		// A budget of 1ms that even a local server plus scheduling overshoots
		// often but not always; assert only that a met budget stays OK.
		{Name: "fast", URL: srv.URL},
	}})
	if got.Status != engine.OK && got.Status != engine.WARN {
		t.Fatalf("a latency budget must yield OK or WARN, never %s: %s", got.Status, got.Message)
	}

	generous := run(t, engine.Flow{Name: "fine", MaxLatencyMS: 60000, Steps: []engine.FlowStep{
		{Name: "fast", URL: srv.URL},
	}})
	if generous.Status != engine.OK {
		t.Fatalf("want OK well inside the budget, got %s: %s", generous.Status, generous.Message)
	}
}

func TestCredentialsComeFromTheEnvironment(t *testing.T) {
	var gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
	}))
	defer srv.Close()

	t.Setenv("CF_TEST_TOKEN", "Bearer env-token")
	t.Setenv("CF_TEST_BODY", "client_secret=env-secret")
	got := run(t, engine.Flow{Name: "env", Steps: []engine.FlowStep{
		{Name: "post", Method: "POST", URL: srv.URL,
			HeadersEnv: map[string]string{"Authorization": "CF_TEST_TOKEN"},
			BodyEnv:    "CF_TEST_BODY"},
	}})
	if got.Status != engine.OK {
		t.Fatalf("want OK, got %s: %s", got.Status, got.Message)
	}
	if gotAuth != "Bearer env-token" || gotBody != "client_secret=env-secret" {
		t.Fatalf("the env values did not reach the request: auth=%q body=%q", gotAuth, gotBody)
	}
	if strings.Contains(got.Message, "env-token") || strings.Contains(got.Message, "env-secret") {
		t.Errorf("a secret from the environment leaked into the finding: %q", got.Message)
	}
}

// An env var named in the config but missing at runtime is a config problem the
// check could not measure past, not a broken target.
func TestAMissingEnvVarIsERROR(t *testing.T) {
	got := run(t, engine.Flow{Name: "env", Steps: []engine.FlowStep{
		{Name: "post", URL: "http://example.invalid",
			HeadersEnv: map[string]string{"Authorization": "CF_TEST_ABSENT"}},
	}})
	if got.Status != engine.ERROR {
		t.Fatalf("want ERROR, got %s: %s", got.Status, got.Message)
	}
	if !strings.Contains(got.Message, "CF_TEST_ABSENT") {
		t.Errorf("the message must name the variable: %q", got.Message)
	}
}

func TestAnUnresolvedReferenceNamesTheValue(t *testing.T) {
	got := run(t, engine.Flow{Name: "typo", Steps: []engine.FlowStep{
		{Name: "use", URL: "http://example.invalid", Headers: map[string]string{"X-Token": "{{tokne}}"}},
	}})
	if got.Status != engine.ERROR {
		t.Fatalf("want ERROR, got %s: %s", got.Status, got.Message)
	}
	if !strings.Contains(got.Message, `"tokne"`) {
		t.Errorf("the message must name the missing capture: %q", got.Message)
	}
}

func TestAFlowWithNoStepsIsAnError(t *testing.T) {
	got := run(t, engine.Flow{Name: "empty"})
	if got.Status != engine.ERROR || !strings.Contains(got.Message, "no steps") {
		t.Fatalf("want an ERROR naming the empty flow, got %s: %s", got.Status, got.Message)
	}
}

func TestNameAndRunAreWiredUp(t *testing.T) {
	c := New(engine.FlowConfig{})
	if c.Name() != "flow" {
		t.Errorf("want the module name flow, got %q", c.Name())
	}
	if got := c.Run(context.Background()); len(got) != 0 {
		t.Errorf("no flows configured should yield no findings, got %+v", got)
	}
}
