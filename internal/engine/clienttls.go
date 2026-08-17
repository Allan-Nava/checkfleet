package engine

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

// ClientTLS carries the client certificate a module presents during the TLS
// handshake, and the CA it verifies the server against (CF-183).
//
// Before this, no module could present one: in a fleet where mTLS is mandatory
// the affected checks did not connect at all — a binary no, not a degradation.
//
// Embedded into the module configs that build their own tls.Config. The three
// modules whose driver owns the connection string (postgres, mongodb, mysql)
// take the same thing through the DSN/URI instead, and do not repeat these keys:
// a second way to say it is a knob that can disagree with the first.
//
// Paths only, never inline PEM. A private key pasted into checkfleet.yml would
// be a secret in a config file, which the project's own rule forbids.
type ClientTLS struct {
	ClientCert string `yaml:"client_cert"` // PEM certificate presented to the server
	ClientKey  string `yaml:"client_key"`  // PEM private key for that certificate
	CACert     string `yaml:"ca_cert"`     // PEM CA bundle used to verify the server
}

// Set reports whether anything was configured, so callers can keep their
// existing behaviour untouched when it was not.
func (c ClientTLS) Set() bool {
	return c.ClientCert != "" || c.ClientKey != "" || c.CACert != ""
}

// Apply returns base with the configured certificate and CA installed. base may
// be nil. The returned config is a copy: modules share their base config across
// targets, and mutating it in place would leak one target's identity into the
// next.
//
// A half-configured pair is an error rather than a silent fallback to no client
// certificate: "I configured mTLS and the check still says the server refused
// me" is a much worse hour than a startup error naming the missing key.
func (c ClientTLS) Apply(base *tls.Config) (*tls.Config, error) {
	if !c.Set() {
		return base, nil
	}
	var out *tls.Config
	if base != nil {
		out = base.Clone()
	} else {
		out = &tls.Config{MinVersion: tls.VersionTLS12}
	}

	switch {
	case c.ClientCert != "" && c.ClientKey == "":
		return nil, fmt.Errorf("client_cert is set but client_key is missing")
	case c.ClientKey != "" && c.ClientCert == "":
		return nil, fmt.Errorf("client_key is set but client_cert is missing")
	case c.ClientCert != "":
		pair, err := tls.LoadX509KeyPair(c.ClientCert, c.ClientKey)
		if err != nil {
			return nil, fmt.Errorf("load client certificate: %w", err)
		}
		out.Certificates = []tls.Certificate{pair}
	}

	if c.CACert != "" {
		pem, err := os.ReadFile(c.CACert)
		if err != nil {
			return nil, fmt.Errorf("read ca_cert: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("ca_cert %s holds no usable certificate", c.CACert)
		}
		out.RootCAs = pool
		// A CA was named on purpose, so verify against it. Leaving
		// InsecureSkipVerify on here would make the setting decorative.
		out.InsecureSkipVerify = false
	}
	return out, nil
}
