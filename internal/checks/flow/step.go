package flow

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// measureErr marks a failure where the check could not measure at all — a
// network error, an unloadable request — as opposed to a target that answered
// and answered wrong. The distinction is the ERROR/BAD split the whole project
// rests on, and collapsing it would report a broken network path as a broken
// service.
type measureErr struct{ err error }

func (e measureErr) Error() string { return e.err.Error() }
func (e measureErr) Unwrap() error { return e.err }

func statusFor(err error) engine.Status {
	var m measureErr
	if errors.As(err, &m) {
		return engine.ERROR
	}
	return engine.BAD
}

// step performs one request, checks the expectations and captures whatever the
// step extracts. It returns how long the request took.
func (c *Check) step(ctx context.Context, client *http.Client, s engine.FlowStep, captured map[string]string) (time.Duration, error) {
	if s.URL == "" {
		return 0, measureErr{errors.New("no url")}
	}
	url, err := substitute(s.URL, captured)
	if err != nil {
		return 0, measureErr{err}
	}

	body, err := stepBody(s, captured)
	if err != nil {
		return 0, measureErr{err}
	}
	method := s.Method
	if method == "" {
		method = http.MethodGet
	}

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(method), url, reader)
	if err != nil {
		return 0, measureErr{err}
	}
	req.Header.Set("User-Agent", "checkfleet")
	if err := applyHeaders(req, s, captured); err != nil {
		return 0, measureErr{err}
	}

	start := time.Now()
	res, err := client.Do(req)
	took := time.Since(start)
	if err != nil {
		// The URL is not echoed: it may carry a captured value substituted into
		// a path or a query string.
		return took, measureErr{fmt.Errorf("request failed: %w", redactURL(err, url))}
	}
	defer res.Body.Close()
	resBody, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return took, measureErr{fmt.Errorf("reading the response: %w", err)}
	}

	want := s.ExpectStatus
	if want == 0 {
		want = http.StatusOK
	}
	if res.StatusCode != want {
		return took, fmt.Errorf("expected status %d, got %d", want, res.StatusCode)
	}
	if s.ExpectBody != "" && !strings.Contains(string(resBody), s.ExpectBody) {
		// The expected substring comes from the config and is safe to echo; the
		// body is not, so it never appears.
		return took, fmt.Errorf("the response does not contain %q", s.ExpectBody)
	}

	for name, selector := range s.Extract {
		value, err := extract(selector, res, resBody)
		if err != nil {
			// The selector is config, the value is not: this says what was
			// looked for, never what was found.
			return took, fmt.Errorf("extracting %q with %q: %w", name, selector, err)
		}
		captured[name] = value
	}
	return took, nil
}

// stepBody resolves the request body, from the config or from an env var when
// it carries a secret.
func stepBody(s engine.FlowStep, captured map[string]string) (string, error) {
	if s.BodyEnv != "" {
		v := os.Getenv(s.BodyEnv)
		if v == "" {
			return "", fmt.Errorf("body_env %s is empty or unset", s.BodyEnv)
		}
		// Not substituted: an env-provided body is a credential payload, and
		// running a template over it would be a way to leak it into a URL.
		return v, nil
	}
	if s.Body == "" {
		return "", nil
	}
	return substitute(s.Body, captured)
}

// applyHeaders sets the step's headers, resolving {{captured}} references and
// env-provided values.
func applyHeaders(req *http.Request, s engine.FlowStep, captured map[string]string) error {
	for k, v := range s.Headers {
		sub, err := substitute(v, captured)
		if err != nil {
			return fmt.Errorf("header %s: %w", k, err)
		}
		req.Header.Set(k, sub)
	}
	for k, env := range s.HeadersEnv {
		v := os.Getenv(env)
		if v == "" {
			return fmt.Errorf("header %s: %s is empty or unset", k, env)
		}
		req.Header.Set(k, v)
	}
	return nil
}

// redactURL keeps a substituted URL out of an error message. A captured token
// can legitimately end up in a path or a query string, and a transport error
// quotes the whole URL back.
func redactURL(err error, url string) error {
	msg := strings.ReplaceAll(err.Error(), url, engine.Redacted)
	return errors.New(msg)
}
