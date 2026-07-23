// Package httpcheck implements the HTTP probe: status code, latency and an
// optional body substring, per target.
package httpcheck

import (
	"context"
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
	return &Check{cfg: cfg, client: &http.Client{}}
}

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
		return engine.Finding{Check: c.Name(), Target: t.URL, Status: engine.ERROR, Message: fmt.Sprintf("richiesta fallita: %v", err)}
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	latency := time.Since(start)

	if res.StatusCode != t.ExpectStatus {
		return engine.Finding{
			Check: c.Name(), Target: t.URL, Status: engine.BAD,
			Message: fmt.Sprintf("HTTP %d (atteso %d), %dms", res.StatusCode, t.ExpectStatus, latency.Milliseconds()),
		}
	}
	if t.ExpectBody != "" && !strings.Contains(string(body), t.ExpectBody) {
		return engine.Finding{
			Check: c.Name(), Target: t.URL, Status: engine.BAD,
			Message: fmt.Sprintf("body senza %q (HTTP %d, %dms)", t.ExpectBody, res.StatusCode, latency.Milliseconds()),
		}
	}
	if t.MaxLatencyMS > 0 && latency.Milliseconds() > int64(t.MaxLatencyMS) {
		return engine.Finding{
			Check: c.Name(), Target: t.URL, Status: engine.WARN,
			Message: fmt.Sprintf("lento: %dms (soglia %dms), HTTP %d", latency.Milliseconds(), t.MaxLatencyMS, res.StatusCode),
		}
	}
	return engine.Finding{
		Check: c.Name(), Target: t.URL, Status: engine.OK,
		Message: fmt.Sprintf("HTTP %d, %dms", res.StatusCode, latency.Milliseconds()),
	}
}
