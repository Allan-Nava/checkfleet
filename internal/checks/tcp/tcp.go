// Package tcp implements a generic TCP reachability check: it connects to each
// target (optionally over TLS), measures the connect latency, and optionally
// checks the server banner. Reachability for anything that speaks TCP.
package tcp

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

type Check struct {
	cfg engine.TCPConfig
	now func() time.Time
}

func New(cfg engine.TCPConfig) *Check { return &Check{cfg: cfg, now: time.Now} }

func (c *Check) Name() string { return "tcp" }

func (c *Check) Run(ctx context.Context) []engine.Finding {
	findings := make([]engine.Finding, len(c.cfg.Targets))
	sem := make(chan struct{}, 16)
	done := make(chan int)
	for i, t := range c.cfg.Targets {
		go func(i int, t engine.TCPTarget) {
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

func (c *Check) probe(ctx context.Context, t engine.TCPTarget) engine.Finding {
	label := t.Name
	if label == "" {
		label = t.Address
	}
	f := engine.Finding{Check: c.Name(), Target: label}

	start := c.now()
	conn, err := c.dial(ctx, t)
	if err != nil {
		f.Status, f.Message = engine.ERROR, fmt.Sprintf("connessione fallita: %v", err)
		return f
	}
	defer conn.Close()
	latency := c.now().Sub(start)

	if t.ExpectBanner != "" {
		if dl, ok := ctx.Deadline(); ok {
			_ = conn.SetReadDeadline(dl)
		}
		buf := make([]byte, 512)
		n, _ := conn.Read(buf)
		if !strings.Contains(string(buf[:n]), t.ExpectBanner) {
			f.Status, f.Message = engine.BAD, fmt.Sprintf("banner atteso %q non trovato", t.ExpectBanner)
			return f
		}
	}
	if t.MaxLatencyMS > 0 && latency > time.Duration(t.MaxLatencyMS)*time.Millisecond {
		f.Status, f.Message = engine.WARN, fmt.Sprintf("connesso in %s (oltre %dms)", latency.Round(time.Millisecond), t.MaxLatencyMS)
		return f
	}
	f.Status, f.Message = engine.OK, fmt.Sprintf("connesso in %s", latency.Round(time.Millisecond))
	return f
}

func (c *Check) dial(ctx context.Context, t engine.TCPTarget) (net.Conn, error) {
	d := net.Dialer{}
	if t.TLS {
		return tls.DialWithDialer(&d, "tcp", t.Address, &tls.Config{ServerName: hostOf(t.Address), InsecureSkipVerify: true})
	}
	return d.DialContext(ctx, "tcp", t.Address)
}

func hostOf(addr string) string {
	if h, _, err := net.SplitHostPort(addr); err == nil {
		return h
	}
	return addr
}
