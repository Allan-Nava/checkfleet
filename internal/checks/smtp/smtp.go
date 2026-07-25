// Package smtp implements an SMTP relay reachability check: it connects to each
// target, reads the greeting, runs EHLO and — when asked — negotiates TLS
// (implicit or via STARTTLS) to inspect the relay certificate's expiry. It never
// sends mail; it only verifies the relay accepts connections and is healthy.
package smtp

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

type Check struct {
	cfg engine.SMTPConfig
	now func() time.Time
}

func New(cfg engine.SMTPConfig) *Check { return &Check{cfg: cfg, now: time.Now} }

func (c *Check) Name() string { return "smtp" }

func (c *Check) Run(ctx context.Context) []engine.Finding {
	findings := make([]engine.Finding, len(c.cfg.Targets))
	sem := make(chan struct{}, 16)
	done := make(chan int)
	for i, t := range c.cfg.Targets {
		go func(i int, t engine.SMTPTarget) {
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

func (c *Check) probe(ctx context.Context, t engine.SMTPTarget) engine.Finding {
	addr := withDefaultPort(t.Address, t.TLS)
	label := t.Name
	if label == "" {
		label = addr
	}
	f := engine.Finding{Check: c.Name(), Target: label}

	start := c.now()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		f.Status, f.Message = engine.ERROR, fmt.Sprintf("connection failed: %v", err)
		return f
	}
	defer conn.Close()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	latency := c.now().Sub(start)

	// A TLS connection whose leaf we can inspect for expiry, if we get one.
	var tlsState *tls.ConnectionState

	// Implicit TLS (smtps): wrap immediately, greeting comes over TLS.
	if t.TLS {
		tconn := tls.Client(conn, &tls.Config{ServerName: hostOf(addr), InsecureSkipVerify: true})
		if dl, ok := ctx.Deadline(); ok {
			_ = tconn.SetDeadline(dl)
		}
		if err := tconn.HandshakeContext(ctx); err != nil {
			f.Status, f.Message = engine.ERROR, fmt.Sprintf("TLS handshake failed: %v", err)
			return f
		}
		st := tconn.ConnectionState()
		tlsState = &st
		conn = tconn
	}

	br := bufio.NewReader(conn)

	// Greeting: must be a 220 line.
	code, greeting, err := readResponse(br)
	if err != nil {
		f.Status, f.Message = engine.ERROR, fmt.Sprintf("reading greeting: %v", err)
		return f
	}
	if code != 220 {
		f.Status, f.Message = engine.BAD, fmt.Sprintf("unexpected greeting %d: %s", code, greeting)
		return f
	}
	if t.ExpectBanner != "" && !strings.Contains(greeting, t.ExpectBanner) {
		f.Status, f.Message = engine.BAD, fmt.Sprintf("greeting %q does not contain %q", greeting, t.ExpectBanner)
		return f
	}

	// EHLO to enumerate capabilities.
	if _, err := fmt.Fprintf(conn, "EHLO checkfleet\r\n"); err != nil {
		f.Status, f.Message = engine.ERROR, fmt.Sprintf("EHLO write failed: %v", err)
		return f
	}
	code, ehlo, err := readResponse(br)
	if err != nil {
		f.Status, f.Message = engine.ERROR, fmt.Sprintf("reading EHLO: %v", err)
		return f
	}
	if code != 250 {
		f.Status, f.Message = engine.BAD, fmt.Sprintf("EHLO rejected %d: %s", code, ehlo)
		return f
	}
	offersStartTLS := strings.Contains(strings.ToUpper(ehlo), "STARTTLS")

	// STARTTLS: only meaningful on a plain connection.
	if t.StartTLS && !t.TLS {
		if !offersStartTLS {
			f.Status, f.Message = engine.BAD, "STARTTLS required but not offered"
			return f
		}
		if _, err := fmt.Fprintf(conn, "STARTTLS\r\n"); err != nil {
			f.Status, f.Message = engine.ERROR, fmt.Sprintf("STARTTLS write failed: %v", err)
			return f
		}
		if code, resp, err := readResponse(br); err != nil {
			f.Status, f.Message = engine.ERROR, fmt.Sprintf("STARTTLS response: %v", err)
			return f
		} else if code != 220 {
			f.Status, f.Message = engine.BAD, fmt.Sprintf("STARTTLS refused %d: %s", code, resp)
			return f
		}
		tconn := tls.Client(conn, &tls.Config{ServerName: hostOf(addr), InsecureSkipVerify: true})
		if dl, ok := ctx.Deadline(); ok {
			_ = tconn.SetDeadline(dl)
		}
		if err := tconn.HandshakeContext(ctx); err != nil {
			f.Status, f.Message = engine.ERROR, fmt.Sprintf("STARTTLS handshake failed: %v", err)
			return f
		}
		st := tconn.ConnectionState()
		tlsState = &st
		conn = tconn
	}

	// Be a good client: QUIT (best effort, we never sent a message).
	_, _ = fmt.Fprintf(conn, "QUIT\r\n")

	// Certificate expiry, when we negotiated TLS.
	if tlsState != nil {
		if len(tlsState.PeerCertificates) == 0 {
			f.Status, f.Message = engine.ERROR, "no certificate presented"
			return f
		}
		leaf := tlsState.PeerCertificates[0]
		days := int(leaf.NotAfter.Sub(c.now()).Hours() / 24)
		switch {
		case days < 0:
			f.Status = engine.BAD
			f.Message = fmt.Sprintf("relay cert EXPIRED %d days ago (%s, CN=%s)", -days, leaf.NotAfter.Format("2006-01-02"), leaf.Subject.CommonName)
			return f
		case days < c.cfg.CritDays:
			f.Status = engine.BAD
			f.Message = fmt.Sprintf("relay cert expires in %d days (%s, CN=%s)", days, leaf.NotAfter.Format("2006-01-02"), leaf.Subject.CommonName)
			return f
		case days < c.cfg.WarnDays:
			f.Status = engine.WARN
			f.Message = fmt.Sprintf("relay cert expires in %d days (%s, CN=%s)", days, leaf.NotAfter.Format("2006-01-02"), leaf.Subject.CommonName)
			return f
		}
	}

	// Latency is the last thing that can knock an otherwise-healthy relay to WARN.
	if t.MaxLatencyMS > 0 && latency > time.Duration(t.MaxLatencyMS)*time.Millisecond {
		f.Status, f.Message = engine.WARN, fmt.Sprintf("connected in %s (over %dms)", latency.Round(time.Millisecond), t.MaxLatencyMS)
		return f
	}

	f.Status, f.Message = engine.OK, fmt.Sprintf("accepts connections, EHLO ok (%s)", latency.Round(time.Millisecond))
	return f
}

// readResponse reads one (possibly multi-line) SMTP reply and returns its status
// code and the joined text. Continuation lines have a '-' after the code; the
// final line has a space.
func readResponse(br *bufio.Reader) (int, string, error) {
	var lines []string
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return 0, "", err
		}
		line = strings.TrimRight(line, "\r\n")
		if len(line) < 3 {
			return 0, line, fmt.Errorf("malformed reply %q", line)
		}
		lines = append(lines, strings.TrimSpace(line[3:]))
		// The 4th byte is '-' for a continuation, ' ' for the last line.
		if len(line) == 3 || line[3] != '-' {
			code, err := strconv.Atoi(line[:3])
			if err != nil {
				return 0, line, fmt.Errorf("non-numeric reply code %q", line[:3])
			}
			return code, strings.Join(lines, " "), nil
		}
	}
}

// withDefaultPort adds the default SMTP port to a host without one: 465 for
// implicit TLS (smtps), 25 otherwise.
func withDefaultPort(addr string, implicitTLS bool) string {
	if _, _, err := net.SplitHostPort(addr); err == nil {
		return addr
	}
	if implicitTLS {
		return addr + ":465"
	}
	return addr + ":25"
}

func hostOf(addr string) string {
	if h, _, err := net.SplitHostPort(addr); err == nil {
		return h
	}
	return addr
}
