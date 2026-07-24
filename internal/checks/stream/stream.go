// Package stream implements a health check for HLS and DASH streams from their
// manifests: the manifest is reachable and parseable, the bitrate ladder has
// the expected number of renditions, and — for live streams — the live edge is
// fresh (not stalled). It fetches only manifests over HTTP; it never downloads
// media segments.
package stream

import (
	"bufio"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

type Check struct {
	cfg    engine.StreamConfig
	client *http.Client
	// now is injectable for tests.
	now func() time.Time
}

func New(cfg engine.StreamConfig) *Check {
	return &Check{cfg: cfg, client: &http.Client{}, now: time.Now}
}

func (c *Check) Name() string { return "stream" }

func (c *Check) Run(ctx context.Context) []engine.Finding {
	perTarget := make([][]engine.Finding, len(c.cfg.Targets))
	sem := make(chan struct{}, 16)
	done := make(chan int)
	for i, t := range c.cfg.Targets {
		go func(i int, t engine.StreamTarget) {
			sem <- struct{}{}
			perTarget[i] = c.probe(ctx, t)
			<-sem
			done <- i
		}(i, t)
	}
	for range c.cfg.Targets {
		<-done
	}
	var findings []engine.Finding
	for _, fs := range perTarget {
		findings = append(findings, fs...)
	}
	return findings
}

func (c *Check) probe(ctx context.Context, t engine.StreamTarget) []engine.Finding {
	label := t.Name
	if label == "" {
		label = t.URL
	}
	body, ctype, err := c.fetch(ctx, t.URL)
	if err != nil {
		return []engine.Finding{{Check: c.Name(), Target: label, Status: engine.ERROR, Message: fmt.Sprintf("manifest not reachable: %v", err)}}
	}
	if isDASH(t.URL, ctype, body) {
		return c.probeDASH(label, t, body)
	}
	return c.probeHLS(ctx, label, t, body)
}

// ---------- HLS ----------

func (c *Check) probeHLS(ctx context.Context, label string, t engine.StreamTarget, body string) []engine.Finding {
	pl, err := parseM3U8(body, t.URL)
	if err != nil {
		return []engine.Finding{{Check: c.Name(), Target: label, Status: engine.BAD, Message: "invalid HLS manifest: " + err.Error()}}
	}
	var findings []engine.Finding

	if pl.master {
		findings = append(findings, engine.Finding{
			Check: c.Name(), Target: label, Status: engine.OK,
			Message: fmt.Sprintf("HLS master, %d variants", len(pl.variants)),
		})
		findings = append(findings, c.ladderFinding(label, t, len(pl.variants))...)
		if t.Live {
			// Live-edge freshness needs a media playlist: fetch the top variant.
			if v, ok := topVariant(pl.variants); ok {
				if vbody, _, err := c.fetch(ctx, v.uri); err == nil {
					if vpl, err := parseM3U8(vbody, v.uri); err == nil {
						findings = append(findings, c.liveEdgeFinding(label, t, vpl))
					}
				}
			}
		}
		return findings
	}

	// Media playlist.
	kind := "media playlist"
	if pl.endList {
		kind += " (VOD)"
	} else {
		kind += " (live)"
	}
	findings = append(findings, engine.Finding{
		Check: c.Name(), Target: label, Status: engine.OK,
		Message: fmt.Sprintf("%s, %d segments", kind, len(pl.segments)),
	})
	if t.Live {
		findings = append(findings, c.liveEdgeFinding(label, t, pl))
	}
	return findings
}

func (c *Check) ladderFinding(label string, t engine.StreamTarget, n int) []engine.Finding {
	if t.MinVariants <= 0 {
		return nil
	}
	target := label + " [ladder]"
	switch {
	case n == 0:
		return []engine.Finding{{Check: c.Name(), Target: target, Status: engine.BAD, Message: "no variant in the master playlist"}}
	case n < t.MinVariants:
		return []engine.Finding{{Check: c.Name(), Target: target, Status: engine.WARN, Message: fmt.Sprintf("incomplete ladder: %d/%d expected variants", n, t.MinVariants)}}
	default:
		return []engine.Finding{{Check: c.Name(), Target: target, Status: engine.OK, Message: fmt.Sprintf("complete ladder: %d variants", n)}}
	}
}

func (c *Check) liveEdgeFinding(label string, t engine.StreamTarget, pl playlist) engine.Finding {
	target := label + " [live-edge]"
	if pl.endList {
		return engine.Finding{Check: c.Name(), Target: target, Status: engine.WARN, Message: "expected live but the manifest is VOD (#EXT-X-ENDLIST present)"}
	}
	edge, ok := pl.liveEdge()
	if !ok {
		return engine.Finding{Check: c.Name(), Target: target, Status: engine.WARN, Message: "freshness not measurable: no #EXT-X-PROGRAM-DATE-TIME"}
	}
	age := int(c.now().Sub(edge).Seconds())
	msg := fmt.Sprintf("live-edge %ds old", age)
	switch {
	case t.MaxAgeCritSeconds > 0 && age >= t.MaxAgeCritSeconds:
		return engine.Finding{Check: c.Name(), Target: target, Status: engine.BAD, Message: msg + fmt.Sprintf(" (crit threshold %ds): stream stalled?", t.MaxAgeCritSeconds)}
	case t.MaxAgeWarnSeconds > 0 && age >= t.MaxAgeWarnSeconds:
		return engine.Finding{Check: c.Name(), Target: target, Status: engine.WARN, Message: msg + fmt.Sprintf(" (threshold %ds)", t.MaxAgeWarnSeconds)}
	default:
		return engine.Finding{Check: c.Name(), Target: target, Status: engine.OK, Message: msg}
	}
}

// ---------- DASH ----------

type mpd struct {
	Type        string `xml:"type,attr"`
	PublishTime string `xml:"publishTime,attr"`
	Periods     []struct {
		AdaptationSets []struct {
			Representations []struct {
				Bandwidth int `xml:"bandwidth,attr"`
			} `xml:"Representation"`
		} `xml:"AdaptationSet"`
	} `xml:"Period"`
}

func (c *Check) probeDASH(label string, t engine.StreamTarget, body string) []engine.Finding {
	var m mpd
	if err := xml.Unmarshal([]byte(body), &m); err != nil {
		return []engine.Finding{{Check: c.Name(), Target: label, Status: engine.BAD, Message: "invalid DASH (MPD) manifest: " + err.Error()}}
	}
	reps := 0
	for _, p := range m.Periods {
		for _, as := range p.AdaptationSets {
			reps += len(as.Representations)
		}
	}
	live := m.Type == "dynamic"
	kind := "MPD statico (VOD)"
	if live {
		kind = "MPD dinamico (live)"
	}
	findings := []engine.Finding{{
		Check: c.Name(), Target: label, Status: engine.OK,
		Message: fmt.Sprintf("%s, %d representation", kind, reps),
	}}
	findings = append(findings, c.ladderFinding(label, t, reps)...)

	if t.Live {
		target := label + " [live-edge]"
		if !live {
			findings = append(findings, engine.Finding{Check: c.Name(), Target: target, Status: engine.WARN, Message: "expected live but MPD is static (type != dynamic)"})
		} else if pub, err := time.Parse(time.RFC3339, m.PublishTime); err == nil {
			age := int(c.now().Sub(pub).Seconds())
			msg := fmt.Sprintf("publishTime %ds old", age)
			switch {
			case t.MaxAgeCritSeconds > 0 && age >= t.MaxAgeCritSeconds:
				findings = append(findings, engine.Finding{Check: c.Name(), Target: target, Status: engine.BAD, Message: msg + fmt.Sprintf(" (crit threshold %ds)", t.MaxAgeCritSeconds)})
			case t.MaxAgeWarnSeconds > 0 && age >= t.MaxAgeWarnSeconds:
				findings = append(findings, engine.Finding{Check: c.Name(), Target: target, Status: engine.WARN, Message: msg + fmt.Sprintf(" (threshold %ds)", t.MaxAgeWarnSeconds)})
			default:
				findings = append(findings, engine.Finding{Check: c.Name(), Target: target, Status: engine.OK, Message: msg})
			}
		} else {
			findings = append(findings, engine.Finding{Check: c.Name(), Target: target, Status: engine.WARN, Message: "freshness not measurable: publishTime missing/unreadable"})
		}
	}
	return findings
}

// ---------- shared ----------

func (c *Check) fetch(ctx context.Context, rawurl string) (body, ctype string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawurl, nil)
	if err != nil {
		return "", "", err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", "", err
	}
	return string(b), resp.Header.Get("Content-Type"), nil
}

func isDASH(rawurl, ctype, body string) bool {
	if strings.HasSuffix(strings.ToLower(pathOf(rawurl)), ".mpd") {
		return true
	}
	if strings.Contains(ctype, "dash+xml") {
		return true
	}
	return strings.Contains(body, "<MPD")
}

func pathOf(rawurl string) string {
	if u, err := url.Parse(rawurl); err == nil {
		return u.Path
	}
	return rawurl
}

// ---------- HLS parsing ----------

type variant struct {
	uri       string
	bandwidth int
}

type segment struct {
	duration float64
	pdt      time.Time
	hasPDT   bool
}

type playlist struct {
	master   bool
	endList  bool
	variants []variant
	segments []segment
}

func parseM3U8(body, baseURL string) (playlist, error) {
	var pl playlist
	base, _ := url.Parse(baseURL)
	sc := bufio.NewScanner(strings.NewReader(body))
	sc.Buffer(make([]byte, 0, 64*1024), 8<<20)

	first := true
	var pendingBW int
	var pendingDur float64
	var pendingPDT time.Time
	var pendingHasPDT bool
	expectVariantURI := false
	expectSegmentURI := false

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if first {
			if !strings.HasPrefix(line, "#EXTM3U") {
				return pl, fmt.Errorf("manca #EXTM3U")
			}
			first = false
			continue
		}
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "#EXT-X-STREAM-INF:"):
			pl.master = true
			pendingBW = attrInt(line, "BANDWIDTH")
			expectVariantURI = true
		case strings.HasPrefix(line, "#EXTINF:"):
			pendingDur = extinfDuration(line)
			expectSegmentURI = true
		case strings.HasPrefix(line, "#EXT-X-PROGRAM-DATE-TIME:"):
			if ts, err := time.Parse(time.RFC3339, strings.TrimPrefix(line, "#EXT-X-PROGRAM-DATE-TIME:")); err == nil {
				pendingPDT, pendingHasPDT = ts, true
			}
		case line == "#EXT-X-ENDLIST":
			pl.endList = true
		case strings.HasPrefix(line, "#"):
			// other tags ignored
		default: // a URI line
			if expectVariantURI {
				pl.variants = append(pl.variants, variant{uri: resolve(base, line), bandwidth: pendingBW})
				expectVariantURI = false
			} else if expectSegmentURI {
				pl.segments = append(pl.segments, segment{duration: pendingDur, pdt: pendingPDT, hasPDT: pendingHasPDT})
				expectSegmentURI = false
				pendingHasPDT = false // PDT applies once, then time is derived by duration
			}
		}
	}
	if err := sc.Err(); err != nil {
		return pl, err
	}
	if !pl.master && len(pl.segments) == 0 {
		return pl, fmt.Errorf("neither variants nor segments found")
	}
	return pl, nil
}

// liveEdge returns the wall-clock time of the live edge (start of the manifest
// carried by the first PDT, advanced by every segment's duration).
func (pl playlist) liveEdge() (time.Time, bool) {
	var cur time.Time
	var have bool
	for _, s := range pl.segments {
		if s.hasPDT {
			cur, have = s.pdt, true
		}
		if have {
			cur = cur.Add(time.Duration(s.duration * float64(time.Second)))
		}
	}
	return cur, have
}

func topVariant(vs []variant) (variant, bool) {
	if len(vs) == 0 {
		return variant{}, false
	}
	top := vs[0]
	for _, v := range vs[1:] {
		if v.bandwidth > top.bandwidth {
			top = v
		}
	}
	return top, true
}

func resolve(base *url.URL, ref string) string {
	if base == nil {
		return ref
	}
	if u, err := url.Parse(ref); err == nil {
		return base.ResolveReference(u).String()
	}
	return ref
}

func attrInt(line, key string) int {
	for _, part := range strings.Split(line[strings.Index(line, ":")+1:], ",") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, key+"=") {
			if n, err := strconv.Atoi(strings.TrimPrefix(part, key+"=")); err == nil {
				return n
			}
		}
	}
	return 0
}

func extinfDuration(line string) float64 {
	rest := strings.TrimPrefix(line, "#EXTINF:")
	if i := strings.Index(rest, ","); i >= 0 {
		rest = rest[:i]
	}
	d, _ := strconv.ParseFloat(strings.TrimSpace(rest), 64)
	return d
}
