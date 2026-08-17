package engine

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// certPair writes a throwaway self-signed certificate and key, returning their
// paths. Generated in-test like every other fixture in this repo: a checked-in
// key is a secret in the tree even when it is worthless.
func certPair(t *testing.T) (certPath, keyPath string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "checkfleet-test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certPath = filepath.Join(dir, "c.pem")
	keyPath = filepath.Join(dir, "k.pem")
	kb, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb}), 0o600); err != nil {
		t.Fatal(err)
	}
	return certPath, keyPath
}

func TestClientTLSUnsetIsInert(t *testing.T) {
	base := &tls.Config{ServerName: "x"}
	got, err := ClientTLS{}.Apply(base)
	if err != nil {
		t.Fatal(err)
	}
	if got != base {
		t.Error("an unconfigured ClientTLS must return the base untouched")
	}
	if (ClientTLS{}).Set() {
		t.Error("the zero value is not set")
	}
}

func TestClientTLSLoadsThePair(t *testing.T) {
	cert, key := certPair(t)
	got, err := ClientTLS{ClientCert: cert, ClientKey: key}.Apply(&tls.Config{ServerName: "srv"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Certificates) != 1 {
		t.Fatalf("certificates = %d, want 1", len(got.Certificates))
	}
	if got.ServerName != "srv" {
		t.Error("the base config's fields must survive")
	}
}

// TestClientTLSRejectsAHalfPair — a silent fallback to "no client certificate"
// turns a config typo into an hour of debugging why the server refuses us.
func TestClientTLSRejectsAHalfPair(t *testing.T) {
	cert, key := certPair(t)
	if _, err := (ClientTLS{ClientCert: cert}).Apply(nil); err == nil {
		t.Error("cert without key must be an error")
	} else if !strings.Contains(err.Error(), "client_key") {
		t.Errorf("the error should name the missing key: %v", err)
	}
	if _, err := (ClientTLS{ClientKey: key}).Apply(nil); err == nil {
		t.Error("key without cert must be an error")
	}
}

func TestClientTLSCAEnablesVerification(t *testing.T) {
	cert, _ := certPair(t)
	// A module that skips verification by default must stop doing so once a CA
	// is named on purpose, or the setting is decorative.
	got, err := ClientTLS{CACert: cert}.Apply(&tls.Config{InsecureSkipVerify: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.RootCAs == nil {
		t.Error("ca_cert did not populate the trust store")
	}
	if got.InsecureSkipVerify {
		t.Error("naming a CA must turn verification back on")
	}
}

func TestClientTLSDoesNotMutateTheBase(t *testing.T) {
	// Modules share one base config across targets; mutating it in place would
	// leak one target's identity into the next.
	cert, key := certPair(t)
	base := &tls.Config{ServerName: "shared"}
	if _, err := (ClientTLS{ClientCert: cert, ClientKey: key}).Apply(base); err != nil {
		t.Fatal(err)
	}
	if len(base.Certificates) != 0 {
		t.Error("the base config was mutated in place")
	}
}

func TestClientTLSReportsUnreadableFiles(t *testing.T) {
	if _, err := (ClientTLS{ClientCert: "/nope/c.pem", ClientKey: "/nope/k.pem"}).Apply(nil); err == nil {
		t.Error("a missing certificate must error")
	}
	if _, err := (ClientTLS{CACert: "/nope/ca.pem"}).Apply(nil); err == nil {
		t.Error("a missing CA must error")
	}
	// A file that exists but holds no certificate is the confusing case.
	dir := t.TempDir()
	junk := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(junk, []byte("not a certificate\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := (ClientTLS{CACert: junk}).Apply(nil)
	if err == nil || !strings.Contains(err.Error(), "no usable certificate") {
		t.Errorf("a CA file without a certificate should say so: %v", err)
	}
}

func TestClientTLSDefaultsToTLS12WithoutABase(t *testing.T) {
	cert, key := certPair(t)
	got, err := ClientTLS{ClientCert: cert, ClientKey: key}.Apply(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %x, want TLS 1.2 when we build the config ourselves", got.MinVersion)
	}
}

// TestClientTLSBindsInlineFromYAML guards the config shape. ClientTLS is a
// *named* field with `yaml:",inline"` rather than an embedded one: embedding
// made staticcheck ask for `cfg.Set()` at every call site, which reads as
// nonsense on a KafkaConfig. Named keeps `cfg.ClientTLS.Set()` readable — but
// only if inline binding still works, which is what this asserts.
func TestClientTLSBindsInlineFromYAML(t *testing.T) {
	cfg, err := LoadBytes([]byte(
		"checks:\n  http:\n    client_cert: /a.pem\n    client_key: /b.pem\n    ca_cert: /c.pem\n    targets: []\n"))
	if err != nil {
		t.Fatal(err)
	}
	got := cfg.Checks.HTTP.ClientTLS
	if got.ClientCert != "/a.pem" || got.ClientKey != "/b.pem" || got.CACert != "/c.pem" {
		t.Fatalf("inline yaml did not bind the three keys: %+v", got)
	}
	// And the six modules that take it must all bind it the same way.
	full := "checks:\n"
	for _, m := range []string{"http", "grpc", "tcp", "smtp", "elasticsearch"} {
		full += "  " + m + ":\n    client_cert: /x.pem\n    client_key: /y.pem\n    targets: []\n"
	}
	full += "  kafka:\n    client_cert: /x.pem\n    client_key: /y.pem\n    brokers: []\n"
	c2, err := LoadBytes([]byte(full))
	if err != nil {
		t.Fatal(err)
	}
	for name, ct := range map[string]ClientTLS{
		"http": c2.Checks.HTTP.ClientTLS, "grpc": c2.Checks.GRPC.ClientTLS,
		"tcp": c2.Checks.TCP.ClientTLS, "smtp": c2.Checks.SMTP.ClientTLS,
		"elasticsearch": c2.Checks.Elasticsearch.ClientTLS, "kafka": c2.Checks.Kafka.ClientTLS,
	} {
		if !ct.Set() {
			t.Errorf("%s did not bind client_cert/client_key", name)
		}
	}
}
