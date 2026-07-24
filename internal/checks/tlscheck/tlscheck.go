// Package tlscheck implements a deep TLS check that complements certs (which
// only reads leaf expiry): it validates the presented chain against the trust
// store, reports certificate expiry, and flags weak negotiated protocol
// versions. Read-only handshake.
package tlscheck

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/inventory"
)

type Check struct {
	cfg engine.TLSConfig
	now func() time.Time
	// roots overrides the trust store for chain verification (tests). nil = system.
	roots *x509.CertPool
}

func New(cfg engine.TLSConfig) *Check { return &Check{cfg: cfg, now: time.Now} }

func (c *Check) Name() string { return "tls" }

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
	var findings []engine.Finding
	if err != nil {
		findings = append(findings, engine.Finding{Check: c.Name(), Target: c.cfg.AnsibleInventory, Status: engine.ERROR, Message: err.Error()})
	}
	perTarget := make([][]engine.Finding, len(targets))
	sem := make(chan struct{}, 16)
	done := make(chan int)
	for i, target := range targets {
		go func(i int, target string) {
			sem <- struct{}{}
			perTarget[i] = c.probe(ctx, target)
			<-sem
			done <- i
		}(i, target)
	}
	for range targets {
		<-done
	}
	for _, fs := range perTarget {
		findings = append(findings, fs...)
	}
	return findings
}

func (c *Check) probe(ctx context.Context, target string) []engine.Finding {
	host := hostOf(target)
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "tcp", target)
	if err != nil {
		return []engine.Finding{{Check: c.Name(), Target: target, Status: engine.ERROR, Message: fmt.Sprintf("connection failed: %v", err)}}
	}
	defer conn.Close()
	// InsecureSkipVerify: read the presented chain even if it doesn't validate;
	// we verify it ourselves below and want expiry regardless. MinVersion TLS 1.0
	// so we can still connect to (and then warn about) servers stuck on old TLS.
	tconn := tls.Client(conn, &tls.Config{ServerName: host, InsecureSkipVerify: true, MinVersion: tls.VersionTLS10})
	if dl, ok := ctx.Deadline(); ok {
		_ = tconn.SetDeadline(dl)
	}
	if err := tconn.HandshakeContext(ctx); err != nil {
		return []engine.Finding{{Check: c.Name(), Target: target, Status: engine.ERROR, Message: fmt.Sprintf("TLS handshake failed: %v", err)}}
	}
	state := tconn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return []engine.Finding{{Check: c.Name(), Target: target, Status: engine.ERROR, Message: "no certificate presented"}}
	}

	return []engine.Finding{
		c.expiryFinding(target, state.PeerCertificates[0]),
		c.chainFinding(target, host, state.PeerCertificates),
		c.protocolFinding(target, state.Version),
	}
}

func (c *Check) expiryFinding(target string, leaf *x509.Certificate) engine.Finding {
	f := engine.Finding{Check: c.Name(), Target: target + " [expiry]"}
	days := int(leaf.NotAfter.Sub(c.now()).Hours() / 24)
	switch {
	case days < 0:
		f.Status, f.Message = engine.BAD, fmt.Sprintf("EXPIRED %d days ago (%s)", -days, leaf.NotAfter.Format("2006-01-02"))
	case days < c.cfg.CritDays:
		f.Status, f.Message = engine.BAD, fmt.Sprintf("expires in %d days (%s)", days, leaf.NotAfter.Format("2006-01-02"))
	case days < c.cfg.WarnDays:
		f.Status, f.Message = engine.WARN, fmt.Sprintf("expires in %d days (%s)", days, leaf.NotAfter.Format("2006-01-02"))
	default:
		f.Status, f.Message = engine.OK, fmt.Sprintf("expires in %d days (%s)", days, leaf.NotAfter.Format("2006-01-02"))
	}
	return f
}

func (c *Check) chainFinding(target, host string, chain []*x509.Certificate) engine.Finding {
	f := engine.Finding{Check: c.Name(), Target: target + " [chain]"}
	inter := x509.NewCertPool()
	for _, cert := range chain[1:] {
		inter.AddCert(cert)
	}
	_, err := chain[0].Verify(x509.VerifyOptions{
		DNSName:       host,
		Roots:         c.roots,
		Intermediates: inter,
		CurrentTime:   c.now(),
	})
	if err != nil {
		f.Status, f.Message = engine.BAD, "invalid chain: "+err.Error()
		return f
	}
	f.Status, f.Message = engine.OK, fmt.Sprintf("valid chain (CN=%s)", chain[0].Subject.CommonName)
	return f
}

func (c *Check) protocolFinding(target string, version uint16) engine.Finding {
	f := engine.Finding{Check: c.Name(), Target: target + " [protocol]"}
	name := tlsVersionName(version)
	if version < tls.VersionTLS12 {
		f.Status, f.Message = engine.WARN, "weak TLS version negotiated: "+name
	} else {
		f.Status, f.Message = engine.OK, name
	}
	return f
}

func tlsVersionName(v uint16) string {
	switch v {
	case tls.VersionTLS13:
		return "TLS 1.3"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS10:
		return "TLS 1.0"
	default:
		return fmt.Sprintf("0x%04x", v)
	}
}

func hostOf(addr string) string {
	if h, _, err := net.SplitHostPort(addr); err == nil {
		return h
	}
	return addr
}

func withDefaultPort(target string, port int) string {
	if strings.Contains(target, ":") {
		return target
	}
	return fmt.Sprintf("%s:%d", target, port)
}
