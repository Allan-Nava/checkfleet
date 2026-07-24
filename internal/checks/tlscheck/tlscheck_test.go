package tlscheck

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// genCA returns a self-signed CA cert/key and a pool containing it.
func genCA(t *testing.T) (*x509.Certificate, *ecdsa.PrivateKey, *x509.CertPool) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "checkfleet-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return cert, key, pool
}

// startTLS serves a leaf signed by the CA, valid for `days`, on 127.0.0.1.
func startTLS(t *testing.T, caCert *x509.Certificate, caKey *ecdsa.PrivateKey, days int, maxVer uint16) string {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Duration(days) * 24 * time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	cert := tls.Certificate{Certificate: [][]byte{der, caCert.Raw}, PrivateKey: key}
	cfg := &tls.Config{Certificates: []tls.Certificate{cert}}
	if maxVer != 0 {
		cfg.MaxVersion = maxVer
		cfg.MinVersion = tls.VersionTLS10 // otherwise default min (1.2) > max → no overlap
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) { _ = c.(*tls.Conn).Handshake(); c.Close() }(conn)
		}
	}()
	return ln.Addr().String()
}

func run(t *testing.T, roots *x509.CertPool, target string) map[string]engine.Finding {
	t.Helper()
	c := New(engine.TLSConfig{Targets: []string{target}, WarnDays: 30, CritDays: 7})
	c.roots = roots
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	m := map[string]engine.Finding{}
	for _, f := range c.Run(ctx) {
		m[f.Target] = f
	}
	return m
}

func TestHealthyChainExpiryProtocol(t *testing.T) {
	ca, key, pool := genCA(t)
	addr := startTLS(t, ca, key, 100, 0)
	f := run(t, pool, addr)
	if f[addr+" [chain]"].Status != engine.OK {
		t.Errorf("catena valida: atteso OK, avuto %s (%s)", f[addr+" [chain]"].Status, f[addr+" [chain]"].Message)
	}
	if f[addr+" [expiry]"].Status != engine.OK {
		t.Errorf("scadenza 100gg: atteso OK, avuto %s", f[addr+" [expiry]"].Status)
	}
	if f[addr+" [protocol]"].Status != engine.OK {
		t.Errorf("protocollo moderno: atteso OK, avuto %s (%s)", f[addr+" [protocol]"].Status, f[addr+" [protocol]"].Message)
	}
}

func TestExpiringSoonIsBad(t *testing.T) {
	ca, key, pool := genCA(t)
	addr := startTLS(t, ca, key, 3, 0) // < crit 7
	if got := run(t, pool, addr)[addr+" [expiry]"]; got.Status != engine.BAD {
		t.Errorf("scade tra 3gg: atteso BAD, avuto %s (%s)", got.Status, got.Message)
	}
}

func TestUntrustedChainIsBad(t *testing.T) {
	ca, key, _ := genCA(t)
	addr := startTLS(t, ca, key, 100, 0)
	// Verify against an empty pool → chain untrusted.
	if got := run(t, x509.NewCertPool(), addr)[addr+" [chain]"]; got.Status != engine.BAD {
		t.Errorf("catena non fidata: atteso BAD, avuto %s (%s)", got.Status, got.Message)
	}
}

func TestWeakProtocolIsWarn(t *testing.T) {
	ca, key, pool := genCA(t)
	addr := startTLS(t, ca, key, 100, tls.VersionTLS11) // server max TLS 1.1
	if got := run(t, pool, addr)[addr+" [protocol]"]; got.Status != engine.WARN {
		t.Errorf("TLS 1.1: atteso WARN, avuto %s (%s)", got.Status, got.Message)
	}
}

func TestUnreachableIsError(t *testing.T) {
	if got := run(t, nil, "127.0.0.1:1")["127.0.0.1:1"]; got.Status != engine.ERROR {
		t.Errorf("irraggiungibile: atteso ERROR, avuto %s (%s)", got.Status, got.Message)
	}
}
