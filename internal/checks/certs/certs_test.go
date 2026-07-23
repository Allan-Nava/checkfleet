package certs

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

// startTLSServer spins a TLS listener presenting a cert that expires in
// `days` days and returns its address.
func startTLSServer(t *testing.T, days int) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "checkfleet-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Duration(days) * 24 * time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}

	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
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
			go func(c net.Conn) {
				_ = c.(*tls.Conn).Handshake()
				c.Close()
			}(conn)
		}
	}()
	return ln.Addr().String()
}

func findingFor(t *testing.T, cfg engine.CertsConfig, target string) engine.Finding {
	t.Helper()
	check := New(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return check.probe(ctx, target)
}

func TestCertExpiryStatuses(t *testing.T) {
	cfg := engine.CertsConfig{WarnDays: 30, CritDays: 7, Port: 443}

	okAddr := startTLSServer(t, 100)
	warnAddr := startTLSServer(t, 10)
	badAddr := startTLSServer(t, 2)

	if f := findingFor(t, cfg, okAddr); f.Status != engine.OK {
		t.Errorf("100 giorni: atteso OK, avuto %s (%s)", f.Status, f.Message)
	}
	if f := findingFor(t, cfg, warnAddr); f.Status != engine.WARN {
		t.Errorf("10 giorni: atteso WARN, avuto %s (%s)", f.Status, f.Message)
	}
	if f := findingFor(t, cfg, badAddr); f.Status != engine.BAD {
		t.Errorf("2 giorni: atteso BAD, avuto %s (%s)", f.Status, f.Message)
	}
}

func TestConnectionRefusedIsError(t *testing.T) {
	cfg := engine.CertsConfig{WarnDays: 30, CritDays: 7}
	f := findingFor(t, cfg, "127.0.0.1:1") // porta chiusa
	if f.Status != engine.ERROR {
		t.Errorf("atteso ERROR, avuto %s (%s)", f.Status, f.Message)
	}
}

func TestTargetsFromConfigAndDefaultPort(t *testing.T) {
	check := New(engine.CertsConfig{Port: 443, Targets: []string{"a.example", "b.example:8443"}})
	targets, err := check.Targets()
	if err != nil {
		t.Fatal(err)
	}
	if targets[0] != "a.example:443" || targets[1] != "b.example:8443" {
		t.Errorf("targets sbagliati: %v", targets)
	}
}
