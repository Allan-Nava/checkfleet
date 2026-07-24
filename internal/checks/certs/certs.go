// Package certs implements the TLS certificate expiry check: it connects to
// every target, reads the leaf certificate and reports how many days are left.
package certs

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/inventory"
)

type Check struct {
	cfg engine.CertsConfig
	// now is injectable for tests.
	now func() time.Time
}

func New(cfg engine.CertsConfig) *Check {
	return &Check{cfg: cfg, now: time.Now}
}

func (c *Check) Name() string { return "certs" }

// Targets resolves explicit targets plus inventory hosts to host:port pairs.
func (c *Check) Targets() ([]string, error) {
	var targets []string
	for _, t := range c.cfg.Targets {
		targets = append(targets, withDefaultPort(t, c.cfg.Port))
	}
	if c.cfg.AnsibleInventory != "" {
		hosts, err := inventory.LoadPath(c.cfg.AnsibleInventory)
		if err != nil {
			return targets, fmt.Errorf("inventory %s: %w", c.cfg.AnsibleInventory, err)
		}
		for _, h := range hosts {
			targets = append(targets, withDefaultPort(h.Address, c.cfg.Port))
		}
	}
	return targets, nil
}

func (c *Check) Run(ctx context.Context) []engine.Finding {
	targets, err := c.Targets()
	if err != nil {
		return append(c.probeAll(ctx, targets), engine.Finding{
			Check: c.Name(), Target: c.cfg.AnsibleInventory, Status: engine.ERROR, Message: err.Error(),
		})
	}
	return c.probeAll(ctx, targets)
}

func (c *Check) probeAll(ctx context.Context, targets []string) []engine.Finding {
	findings := make([]engine.Finding, len(targets))
	sem := make(chan struct{}, 16)
	done := make(chan int)
	for i, target := range targets {
		go func(i int, target string) {
			sem <- struct{}{}
			findings[i] = c.probe(ctx, target)
			<-sem
			done <- i
		}(i, target)
	}
	for range targets {
		<-done
	}
	return findings
}

func (c *Check) probe(ctx context.Context, target string) engine.Finding {
	host, _, _ := net.SplitHostPort(target)
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", target)
	if err != nil {
		return engine.Finding{Check: c.Name(), Target: target, Status: engine.ERROR, Message: fmt.Sprintf("connection failed: %v", err)}
	}
	defer conn.Close()

	// InsecureSkipVerify: we want the expiry date even when the chain does not
	// validate locally; the expiry itself is what this check is about.
	tlsConn := tls.Client(conn, &tls.Config{ServerName: host, InsecureSkipVerify: true})
	if deadline, ok := ctx.Deadline(); ok {
		_ = tlsConn.SetDeadline(deadline)
	}
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return engine.Finding{Check: c.Name(), Target: target, Status: engine.ERROR, Message: fmt.Sprintf("TLS handshake failed: %v", err)}
	}
	certs := tlsConn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return engine.Finding{Check: c.Name(), Target: target, Status: engine.ERROR, Message: "no certificate presented"}
	}
	leaf := certs[0]
	days := int(leaf.NotAfter.Sub(c.now()).Hours() / 24)
	msg := fmt.Sprintf("expires in %d days (%s, CN=%s)", days, leaf.NotAfter.Format("2006-01-02"), leaf.Subject.CommonName)

	status := engine.OK
	switch {
	case days < 0:
		status = engine.BAD
		msg = fmt.Sprintf("EXPIRED %d days ago (%s, CN=%s)", -days, leaf.NotAfter.Format("2006-01-02"), leaf.Subject.CommonName)
	case days < c.cfg.CritDays:
		status = engine.BAD
	case days < c.cfg.WarnDays:
		status = engine.WARN
	}
	return engine.Finding{Check: c.Name(), Target: target, Status: status, Message: msg}
}

func withDefaultPort(target string, port int) string {
	if strings.Contains(target, ":") {
		return target
	}
	return fmt.Sprintf("%s:%d", target, port)
}
