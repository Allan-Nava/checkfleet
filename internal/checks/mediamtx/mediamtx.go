// Package mediamtx implements the mediamtx streaming-server check against its
// v3 control API: which paths exist, whether each is ready (someone is
// publishing), how many readers it has, and whether bytes are actually moving
// (CF-21).
//
// This is the layer nothing else can see. The `ingest` module says a streamer
// *can* connect and the `stream` module says a manifest is being served — but
// between them sits the case that takes a broadcast off the air quietly: the
// encoder died, the path went not-ready, and the manifest is still cached
// downstream so every other probe stays green.
package mediamtx

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

type Check struct {
	cfg    engine.MediaMTXConfig
	client *http.Client
	tlsErr error
}

func New(cfg engine.MediaMTXConfig) *Check {
	c := &Check{cfg: cfg, client: &http.Client{}}
	if cfg.ClientTLS.Set() {
		tc, err := cfg.ClientTLS.Apply(&tls.Config{MinVersion: tls.VersionTLS12})
		if err != nil {
			c.tlsErr = err
		} else {
			c.client = &http.Client{Transport: &http.Transport{TLSClientConfig: tc}}
		}
	}
	return c
}

func (c *Check) Name() string { return "mediamtx" }

func (c *Check) Run(ctx context.Context) []engine.Finding {
	var findings []engine.Finding
	for _, t := range c.cfg.Targets {
		findings = append(findings, c.probe(ctx, t)...)
	}
	return findings
}

// pathItem is the slice of /v3/paths/list this check reads. mediamtx returns
// more per path; decoding only these keeps the coupling to its response shape
// as small as it can be.
type pathItem struct {
	Name   string `json:"name"`
	Ready  bool   `json:"ready"`
	Source *struct {
		Type string `json:"type"`
	} `json:"source"`
	Tracks        []string `json:"tracks"`
	BytesReceived int64    `json:"bytesReceived"`
	BytesSent     int64    `json:"bytesSent"`
	Readers       []struct {
		Type string `json:"type"`
	} `json:"readers"`
}

type pathList struct {
	ItemCount int        `json:"itemCount"`
	PageCount int        `json:"pageCount"`
	Items     []pathItem `json:"items"`
}

func (c *Check) probe(ctx context.Context, t engine.MediaMTXTarget) []engine.Finding {
	label := t.Name
	if label == "" {
		label = t.URL
	}
	fail := func(status engine.Status, format string, args ...any) []engine.Finding {
		return []engine.Finding{{Check: c.Name(), Target: label, Status: status,
			Message: fmt.Sprintf(format, args...)}}
	}

	if c.tlsErr != nil {
		return fail(engine.ERROR, "client certificate: %v", c.tlsErr)
	}
	if t.URL == "" {
		return fail(engine.ERROR, "no url")
	}

	start := time.Now()
	list, err := c.paths(ctx, t)
	latency := time.Since(start)
	if err != nil {
		// ERROR, not BAD: an unreachable API means the check could not measure
		// the server, not that the server is serving badly.
		return fail(engine.ERROR, "%v", err)
	}

	byName := map[string]pathItem{}
	ready := 0
	for _, p := range list.Items {
		byName[p.Name] = p
		if p.Ready {
			ready++
		}
	}

	api := engine.Finding{Check: c.Name(), Target: label, Status: engine.OK,
		Message: fmt.Sprintf("API reachable, %d path(s), %d ready", len(list.Items), ready),
		Value:   engine.Num(float64(len(list.Items))), Unit: "paths"}
	if t.MaxLatencyMS > 0 && latency > time.Duration(t.MaxLatencyMS)*time.Millisecond {
		api.Status = engine.WARN
		api.Message = fmt.Sprintf("%s, API answered in %dms (max %dms)",
			api.Message, latency.Milliseconds(), t.MaxLatencyMS)
	}
	findings := []engine.Finding{api}

	// With expect_paths the question is "are these on the air"; without it, the
	// question is only "are the paths that exist healthy" — a server with no
	// paths configured is not a fault, it is an idle server.
	names := t.ExpectPaths
	expected := len(names) > 0
	if !expected {
		names = make([]string, 0, len(byName))
		for n := range byName {
			names = append(names, n)
		}
		sort.Strings(names)
	}

	for _, name := range names {
		findings = append(findings, c.pathFinding(label, name, byName, expected, t))
	}
	return findings
}

// pathFinding judges one path.
func (c *Check) pathFinding(label, name string, byName map[string]pathItem, expected bool, t engine.MediaMTXTarget) engine.Finding {
	f := engine.Finding{Check: c.Name(), Target: label + "/" + name}

	p, ok := byName[name]
	if !ok {
		// Only reachable with expect_paths: the other branch iterates what the
		// server reported.
		f.Status, f.Message = engine.BAD, "the path does not exist on this server"
		return f
	}

	if !p.Ready {
		// The outage this module exists for. A path that is configured but not
		// ready has no publisher: the encoder is gone.
		f.Status, f.Message = engine.BAD, "not ready: no publisher"
		return f
	}

	source := "publisher"
	if p.Source != nil && p.Source.Type != "" {
		source = p.Source.Type
	}
	readers := len(p.Readers)
	f.Value, f.Unit = engine.Num(float64(readers)), "readers"

	switch {
	case p.BytesReceived == 0:
		// Ready but nothing has arrived: the publisher connected and then went
		// silent, which looks identical to a healthy path from the outside.
		f.Status = engine.BAD
		f.Message = fmt.Sprintf("ready via %s but no bytes received: the ingest is stalled", source)
	case t.WarnNoReaders && readers == 0:
		f.Status = engine.WARN
		f.Message = fmt.Sprintf("ready via %s, %s, but nobody is reading it", source, tracks(p))
	default:
		f.Status = engine.OK
		f.Message = fmt.Sprintf("ready via %s, %s, %d reader(s)", source, tracks(p), readers)
	}
	return f
}

func tracks(p pathItem) string {
	if len(p.Tracks) == 0 {
		return "no tracks"
	}
	return strings.Join(p.Tracks, "+")
}

// paths fetches /v3/paths/list, following pagination so a server with more
// paths than one page does not silently report a truncated fleet.
func (c *Check) paths(ctx context.Context, t engine.MediaMTXTarget) (pathList, error) {
	base := strings.TrimSuffix(t.URL, "/")
	var all pathList
	for page := 0; ; page++ {
		url := fmt.Sprintf("%s/v3/paths/list?page=%d", base, page)
		var got pathList
		if err := c.getJSON(ctx, t, url, &got); err != nil {
			return all, err
		}
		all.Items = append(all.Items, got.Items...)
		all.ItemCount = got.ItemCount
		if page+1 >= got.PageCount || got.PageCount == 0 || len(got.Items) == 0 {
			break
		}
	}
	return all, nil
}

func (c *Check) getJSON(ctx context.Context, t engine.MediaMTXTarget, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "checkfleet")
	if t.Username != "" && t.PasswordEnv != "" {
		req.SetBasicAuth(t.Username, os.Getenv(t.PasswordEnv))
	}

	client := c.client
	if t.Insecure {
		client = &http.Client{Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // opt-in per target
		}}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("control API unreachable: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("reading the control API response: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("control API rejected the credentials (HTTP 401)")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("control API returned HTTP %d", resp.StatusCode)
	}
	if err := json.Unmarshal(body, out); err != nil {
		// The body is not echoed: it lists path names and connection ids.
		return fmt.Errorf("the control API response is not the expected JSON: %w", err)
	}
	return nil
}
