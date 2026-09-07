package engine

// FlowConfig configures the multi-step flow check (CF-180).
//
// Half of real infrastructure does not answer a single question. "Login works"
// is: get a token, use it, check the answer — and a probe that only asks the
// first of those reports a healthy service while nobody can log in.
//
// Deliberately NOT a scripting language. The steps are declarative YAML and
// nothing in the config is ever executed: a check that runs code from its own
// config would be a new security surface on a process that already holds the
// credentials of up to 30 production systems.
type FlowConfig struct {
	Flows []Flow `yaml:"flows"`
	// Client certificate for mTLS (CF-183); empty leaves the handshake unchanged.
	ClientTLS ClientTLS `yaml:",inline"`
}

// Flow is one ordered sequence of HTTP steps.
type Flow struct {
	// Name identifies the flow in findings, e.g. "login".
	Name string `yaml:"name"`
	// Steps run in order; the first failure stops the flow, because every later
	// step would fail for the same reason and bury the one that matters.
	Steps []FlowStep `yaml:"steps"`
	// MaxLatencyMS WARNs when the whole flow takes longer than this (0
	// disables). The budget is the flow's, not a step's: a login that takes
	// eight seconds is broken regardless of which hop spent them.
	MaxLatencyMS int `yaml:"max_latency_ms"`
	// KeepCookies carries a cookie jar across the steps, which is what a
	// session login actually needs.
	KeepCookies bool `yaml:"keep_cookies"`
	// Insecure skips TLS verification for this flow's requests.
	Insecure bool `yaml:"insecure_skip_verify"`
}

// FlowStep is one request plus what it must produce.
type FlowStep struct {
	// Name identifies the step in the finding: "the flow is broken" without
	// saying where saves nobody any time.
	Name   string `yaml:"name"`
	Method string `yaml:"method"` // default GET
	URL    string `yaml:"url"`
	// Headers sent with the request. Values may reference captured values as
	// {{name}}; so may URL and Body.
	Headers map[string]string `yaml:"headers"`
	// HeadersEnv sets a header from an environment variable, named here rather
	// than written in the config — the same rule every other module follows for
	// a credential.
	HeadersEnv map[string]string `yaml:"headers_env"`
	Body       string            `yaml:"body"`
	// BodyEnv reads the request body from an environment variable, for a body
	// that carries a client secret.
	BodyEnv string `yaml:"body_env"`

	// ExpectStatus is the status code the step must return (default 200).
	ExpectStatus int `yaml:"expect_status"`
	// ExpectBody is a substring the response body must contain.
	ExpectBody string `yaml:"expect_body"`
	// MaxLatencyMS WARNs when this single step is slower than this (0 disables).
	MaxLatencyMS int `yaml:"max_latency_ms"`

	// Extract captures values for later steps: name -> selector. Three
	// selectors, no more: "json:a.b.0.c" (dotted path, numeric segments index
	// arrays), "header:Name", "regex:pattern" (first capture group).
	//
	// Captured values are NEVER printed. A flow that logs in captures a token,
	// and a finding message travels to a terminal, a CI log and a JSON file.
	Extract map[string]string `yaml:"extract"`
}
