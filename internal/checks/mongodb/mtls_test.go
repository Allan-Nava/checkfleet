package mongodb

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// TestURICarriesTheClientCertificate is the mongodb half of CF-183's finding:
// the module needs no `client_cert` key because the connection URI already
// carries mTLS through the driver's own parameters. A second way to say the
// same thing is a knob that can disagree with the one already there.
func TestURICarriesTheClientCertificate(t *testing.T) {
	dir := t.TempDir()
	// The driver wants the certificate and key concatenated in one file.
	pair := filepath.Join(dir, "client.pem")
	if err := os.WriteFile(pair, []byte(testClientPEM), 0o600); err != nil {
		t.Fatal(err)
	}
	uri := "mongodb://db.example:27017/?tls=true&tlsCertificateKeyFile=" + pair

	o := options.Client().ApplyURI(uri)
	if err := o.Validate(); err != nil {
		t.Fatalf("the driver rejected an mTLS URI: %v", err)
	}
	if o.TLSConfig == nil {
		t.Fatal("no TLS config was built from the URI")
	}
	if len(o.TLSConfig.Certificates) != 1 {
		t.Errorf("client certificate not loaded: %d certificates", len(o.TLSConfig.Certificates))
	}
}

func TestURIWithoutTLSStillParses(t *testing.T) {
	o := options.Client().ApplyURI("mongodb://db.example:27017")
	if err := o.Validate(); err != nil {
		t.Fatal(err)
	}
	if o.TLSConfig != nil {
		t.Error("a plain URI should not build a TLS config")
	}
}

func TestClientPEMFixtureIsWellFormed(t *testing.T) {
	if !strings.Contains(testClientPEM, "BEGIN CERTIFICATE") || !strings.Contains(testClientPEM, "PRIVATE KEY") {
		t.Fatal("the fixture must hold both a certificate and its key")
	}
}
