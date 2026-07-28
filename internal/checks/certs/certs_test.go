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
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
		t.Errorf("100 days: want OK, got %s (%s)", f.Status, f.Message)
	} else if f.Unit != "days" || f.Value == nil || *f.Value < 95 || *f.Value > 100 {
		t.Errorf("100 days: want a ~100 days Value/unit (CF-91), got value=%v unit=%q", f.Value, f.Unit)
	}
	if f := findingFor(t, cfg, warnAddr); f.Status != engine.WARN {
		t.Errorf("10 days: want WARN, got %s (%s)", f.Status, f.Message)
	}
	if f := findingFor(t, cfg, badAddr); f.Status != engine.BAD {
		t.Errorf("2 days: want BAD, got %s (%s)", f.Status, f.Message)
	}
}

func TestConnectionRefusedIsError(t *testing.T) {
	cfg := engine.CertsConfig{WarnDays: 30, CritDays: 7}
	f := findingFor(t, cfg, "127.0.0.1:1") // porta chiusa
	if f.Status != engine.ERROR {
		t.Errorf("want ERROR, got %s (%s)", f.Status, f.Message)
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

// Run is the path the engine actually calls: target expansion (explicit list +
// inventory) then a concurrent sweep. Until CF-158 only probe() was covered, so
// nothing exercised the fan-out or the inventory branch.
func TestRunCoversEveryTargetConcurrently(t *testing.T) {
	a := startTLSServer(t, 40)
	b := startTLSServer(t, 3)
	c := New(engine.CertsConfig{Targets: []string{a, b}, WarnDays: 30, CritDays: 7})

	findings := c.Run(context.Background())
	if len(findings) != 2 {
		t.Fatalf("want one finding per target, got %d: %+v", len(findings), findings)
	}
	// The order of findings must follow the order of the targets, not the order
	// the goroutines happened to finish in — the engine sorts afterwards, and a
	// racy order here would make the output non-deterministic run to run.
	if findings[0].Target != a || findings[1].Target != b {
		t.Errorf("findings must stay in target order, got %q then %q", findings[0].Target, findings[1].Target)
	}
	if findings[0].Status != engine.OK || findings[1].Status != engine.BAD {
		t.Errorf("40 days should be OK and 3 days BAD, got %s and %s", findings[0].Status, findings[1].Status)
	}
}

func TestRunAddsInventoryHosts(t *testing.T) {
	srv := startTLSServer(t, 40)
	host, port, ok := strings.Cut(srv, ":")
	if !ok {
		t.Fatalf("unexpected server address %q", srv)
	}
	inv := filepath.Join(t.TempDir(), "hosts.ini")
	if err := os.WriteFile(inv, []byte("[tls]\nnode1 ansible_host="+host+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := strconv.Atoi(port)
	if err != nil {
		t.Fatal(err)
	}

	c := New(engine.CertsConfig{AnsibleInventory: inv, Port: p, WarnDays: 30, CritDays: 7})
	findings := c.Run(context.Background())
	if len(findings) != 1 {
		t.Fatalf("the inventory host should be probed, got %+v", findings)
	}
	if findings[0].Status != engine.OK {
		t.Errorf("want OK from the inventory host, got %s: %s", findings[0].Status, findings[0].Message)
	}
}

// A broken inventory must not silently mean "nothing to check": that would
// report a healthy fleet from a typo in a path.
func TestRunReportsUnreadableInventory(t *testing.T) {
	c := New(engine.CertsConfig{
		AnsibleInventory: filepath.Join(t.TempDir(), "nope.ini"),
		WarnDays:         30, CritDays: 7,
	})
	findings := c.Run(context.Background())
	if len(findings) != 1 || findings[0].Status != engine.ERROR {
		t.Fatalf("want a single ERROR finding for the unreadable inventory, got %+v", findings)
	}
	if !strings.Contains(findings[0].Message, "nope.ini") {
		t.Errorf("the message must name the file: %q", findings[0].Message)
	}
}
