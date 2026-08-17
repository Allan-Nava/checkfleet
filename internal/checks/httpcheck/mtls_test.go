package httpcheck

import (
	"context"
	"crypto/tls"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// mtlsServer starts an HTTPS server that REQUIRES a client certificate, and
// writes the client pair plus the server CA to disk. This is the shape of the
// environment CF-183 exists for: before it, checkfleet could not connect here
// at all.
func mtlsServer(t *testing.T) (url, clientCert, clientKey, caCert string) {
	t.Helper()
	srv := httptest.NewUnstartedServer(nil)
	srv.TLS = &tls.Config{ClientAuth: tls.RequireAnyClientCert}
	srv.StartTLS()
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	// The server's own certificate doubles as the CA the client trusts.
	caCert = filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(caCert, pemOf(srv.Certificate().Raw), 0o600); err != nil {
		t.Fatal(err)
	}
	// Reuse the httptest certificate as the client identity: the server is set
	// to RequireAnyClientCert, so any valid pair proves the handshake carried
	// one — which is exactly the behaviour under test.
	clientCert, clientKey = writeTestPair(t, dir)
	return srv.URL, clientCert, clientKey, caCert
}

// TestWithoutAClientCertificateTheHandshakeFails is the "before" half: it
// documents that the failure this feature removes is a hard no, not a warning.
func TestWithoutAClientCertificateTheHandshakeFails(t *testing.T) {
	url, _, _, ca := mtlsServer(t)
	cfg := engine.HTTPConfig{
		Targets:   []engine.HTTPTarget{{URL: url, ExpectStatus: 200}},
		ClientTLS: engine.ClientTLS{CACert: ca},
	}
	findings := New(cfg).Run(context.Background())
	if len(findings) != 1 {
		t.Fatalf("got %d findings", len(findings))
	}
	if findings[0].Status != engine.ERROR {
		t.Errorf("status = %s, want ERROR: a server demanding a client certificate must not read as healthy", findings[0].Status)
	}
}

func TestWithAClientCertificateTheCheckConnects(t *testing.T) {
	url, cert, key, ca := mtlsServer(t)
	cfg := engine.HTTPConfig{
		Targets:   []engine.HTTPTarget{{URL: url, ExpectStatus: 200}},
		ClientTLS: engine.ClientTLS{ClientCert: cert, ClientKey: key, CACert: ca},
	}
	findings := New(cfg).Run(context.Background())
	if len(findings) != 1 {
		t.Fatalf("got %d findings", len(findings))
	}
	if findings[0].Status == engine.ERROR {
		t.Errorf("status = %s (%s), want a measured result", findings[0].Status, findings[0].Message)
	}
}

// TestABadCertificatePathIsAnErrorFinding — not a panic, and not a silent
// fallback to no certificate, which would look like the server rejecting us.
func TestABadCertificatePathIsAnErrorFinding(t *testing.T) {
	url, _, _, _ := mtlsServer(t)
	cfg := engine.HTTPConfig{
		Targets:   []engine.HTTPTarget{{URL: url, ExpectStatus: 200}},
		ClientTLS: engine.ClientTLS{ClientCert: "/nope/c.pem", ClientKey: "/nope/k.pem"},
	}
	findings := New(cfg).Run(context.Background())
	if findings[0].Status != engine.ERROR {
		t.Errorf("status = %s, want ERROR", findings[0].Status)
	}
}

func pemOf(der []byte) []byte {
	return pemEncode("CERTIFICATE", der)
}
