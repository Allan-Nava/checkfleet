package postgres

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// writeClientPair generates a throwaway client certificate and key and returns
// their paths, plus the CA file (self-signed, so the same cert serves as CA).
func writeClientPair(t *testing.T) (certPath, keyPath string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "checkfleet"},
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
	certPath = filepath.Join(dir, "client.crt")
	keyPath = filepath.Join(dir, "client.key")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	kb, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb}), 0o600); err != nil {
		t.Fatal(err)
	}
	return certPath, keyPath
}

// TestDSNCarriesTheClientCertificate documents and verifies why the postgres
// module has no `client_cert` config key of its own (CF-183): the DSN already
// carries mTLS through the libpq-compatible parameters pgx understands, so
// adding a second way to say the same thing would be a knob that can disagree
// with the one already there.
//
// Asserted rather than assumed: the parsed config must actually end up with a
// TLS configuration holding the client certificate.
func TestDSNCarriesTheClientCertificate(t *testing.T) {
	cert, key := writeClientPair(t)
	dsn := "host=db.example port=5432 user=checkfleet dbname=postgres sslmode=verify-full" +
		" sslcert=" + cert + " sslkey=" + key + " sslrootcert=" + cert

	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("pgx rejected an mTLS DSN: %v", err)
	}
	if cfg.TLSConfig == nil {
		t.Fatal("no TLS config was built from the DSN")
	}
	if len(cfg.TLSConfig.Certificates) != 1 {
		t.Errorf("client certificate not loaded: %d certificates", len(cfg.TLSConfig.Certificates))
	}
	if cfg.TLSConfig.RootCAs == nil {
		t.Error("sslrootcert did not populate the trust store")
	}
}

// TestDSNWithoutCertificatesStillParses — the ordinary case must not regress
// while the mTLS one is supported.
func TestDSNWithoutCertificatesStillParses(t *testing.T) {
	cfg, err := pgx.ParseConfig("host=db.example user=checkfleet dbname=postgres sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TLSConfig != nil {
		t.Error("sslmode=disable should not build a TLS config")
	}
}
