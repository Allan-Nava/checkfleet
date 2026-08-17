// Package httpcheck implements the HTTP probe: status code, latency and an
// optional body substring, per target.
package httpcheck

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

type Check struct {
	cfg    engine.HTTPConfig
	client *http.Client
}

func New(cfg engine.HTTPConfig) *Check {
	return &Check{cfg: cfg, client: httpClient(cfg)}
}

// httpClient honours a configured client certificate (CF-183). Without one the
// zero-value client is kept, so nothing changes for the ordinary case.
func httpClient(cfg engine.HTTPConfig) *http.Client {
	if !cfg.ClientTLS.Set() {
		return &http.Client{}
	}
	tc, err := cfg.ClientTLS.Apply(&tls.Config{MinVersion: tls.VersionTLS12})
	if err != nil {
		// Surfaced as an ERROR finding on the first request: "the check could
		// not measure" is the honest status for a certificate it cannot load.
		return &http.Client{Transport: &errTransport{err: err}}
	}
	return &http.Client{Transport: &http.Transport{TLSClientConfig: tc}}
}

// errTransport fails every request with a fixed error.
type errTransport struct{ err error }

func (e *errTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, e.err }

func (c *Check) Name() string { return "http" }

func (c *Check) Run(ctx context.Context) []engine.Finding {
	findings := make([]engine.Finding, len(c.cfg.Targets))
	sem := make(chan struct{}, 16)
	done := make(chan int)
	for i, t := range c.cfg.Targets {
		go func(i int, t engine.HTTPTarget) {
			sem <- struct{}{}
			findings[i] = c.probe(ctx, t)
			<-sem
			done <- i
		}(i, t)
	}
	for range c.cfg.Targets {
		<-done
	}
	return findings
}

func (c *Check) probe(ctx context.Context, t engine.HTTPTarget) engine.Finding {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.URL, nil)
	if err != nil {
		return engine.Finding{Check: c.Name(), Target: t.URL, Status: engine.ERROR, Message: err.Error()}
	}
	req.Header.Set("User-Agent", "checkfleet")

	start := time.Now()
	res, err := c.client.Do(req)
	if err != nil {
		return engine.Finding{Check: c.Name(), Target: t.URL, Status: engine.ERROR, Message: fmt.Sprintf("request failed: %v", err)}
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	latency := time.Since(start)
	ms := float64(latency.Microseconds()) / 1000 // latency in ms, for Finding.Value

	if res.StatusCode != t.ExpectStatus {
		return engine.Finding{
			Check: c.Name(), Target: t.URL, Status: engine.BAD, Value: engine.Num(ms), Unit: "ms",
			Message: fmt.Sprintf("HTTP %d (want %d), %dms", res.StatusCode, t.ExpectStatus, latency.Milliseconds()),
		}
	}
	if t.ExpectBody != "" && !strings.Contains(string(body), t.ExpectBody) {
		return engine.Finding{
			Check: c.Name(), Target: t.URL, Status: engine.BAD, Value: engine.Num(ms), Unit: "ms",
			Message: fmt.Sprintf("body missing %q (HTTP %d, %dms)", t.ExpectBody, res.StatusCode, latency.Milliseconds()),
		}
	}
	if t.MaxLatencyMS > 0 && latency.Milliseconds() > int64(t.MaxLatencyMS) {
		return engine.Finding{
			Check: c.Name(), Target: t.URL, Status: engine.WARN, Value: engine.Num(ms), Unit: "ms",
			Message: fmt.Sprintf("slow: %dms (threshold %dms), HTTP %d", latency.Milliseconds(), t.MaxLatencyMS, res.StatusCode),
		}
	}
	return engine.Finding{
		Check: c.Name(), Target: t.URL, Status: engine.OK, Value: engine.Num(ms), Unit: "ms",
		Message: fmt.Sprintf("HTTP %d, %dms", res.StatusCode, latency.Milliseconds()),
	}
}
