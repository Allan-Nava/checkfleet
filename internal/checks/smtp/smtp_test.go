package smtp

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// genCert builds a self-signed cert expiring in `days` days.
func genCert(t *testing.T, days int) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "relay-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Duration(days) * 24 * time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// fakeSMTP starts an in-test SMTP server driven by opts and returns its address.
type opts struct {
	greeting    string          // greeting line without CRLF; default "220 relay ESMTP"
	ehlo        []string        // EHLO reply lines (without codes); default {"relay", "STARTTLS"}
	offerTLS    bool            // include STARTTLS in EHLO (overrides ehlo default)
	startTLS    bool            // honor STARTTLS by upgrading with tlsCert
	implicitTLS bool            // wrap the connection in TLS immediately (smtps)
	tlsCert     *tls.Certificate
}

func fakeSMTP(t *testing.T, o opts) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	greeting := o.greeting
	if greeting == "" {
		greeting = "220 relay ESMTP"
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serve(conn, o, greeting)
		}
	}()
	return ln.Addr().String()
}

func serve(conn net.Conn, o opts, greeting string) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))

	if o.implicitTLS && o.tlsCert != nil {
		tc := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{*o.tlsCert}})
		if err := tc.Handshake(); err != nil {
			return
		}
		conn = tc
	}

	w := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
	w(greeting)

	br := bufio.NewReader(conn)
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(cmd, "EHLO"):
			lines := o.ehlo
			if lines == nil {
				lines = []string{"relay greets you"}
				if o.offerTLS {
					lines = append(lines, "STARTTLS")
				}
			}
			for i, l := range lines {
				sep := "-"
				if i == len(lines)-1 {
					sep = " "
				}
				w("250" + sep + l)
			}
		case strings.HasPrefix(cmd, "STARTTLS"):
			if o.startTLS && o.tlsCert != nil {
				w("220 go ahead")
				tc := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{*o.tlsCert}})
				if err := tc.Handshake(); err != nil {
					return
				}
				conn = tc
				br = bufio.NewReader(conn)
				w = func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
			} else {
				w("454 TLS not available")
			}
		case strings.HasPrefix(cmd, "QUIT"):
			w("221 bye")
			return
		default:
			w("250 ok")
		}
	}
}

func run(t *testing.T, cfg engine.SMTPConfig, target engine.SMTPTarget) engine.Finding {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return New(cfg).probe(ctx, target)
}

func TestPlainRelayOK(t *testing.T) {
	addr := fakeSMTP(t, opts{offerTLS: true})
	f := run(t, engine.SMTPConfig{WarnDays: 30, CritDays: 7}, engine.SMTPTarget{Address: addr})
	if f.Status != engine.OK {
		t.Fatalf("want OK, got %s: %s", f.Status, f.Message)
	}
}

func TestUnexpectedGreetingIsBad(t *testing.T) {
	addr := fakeSMTP(t, opts{greeting: "554 no service here"})
	f := run(t, engine.SMTPConfig{WarnDays: 30, CritDays: 7}, engine.SMTPTarget{Address: addr})
	if f.Status != engine.BAD {
		t.Fatalf("want BAD, got %s: %s", f.Status, f.Message)
	}
}

func TestExpectBannerMismatchIsBad(t *testing.T) {
	addr := fakeSMTP(t, opts{greeting: "220 other-host ESMTP"})
	f := run(t, engine.SMTPConfig{WarnDays: 30, CritDays: 7},
		engine.SMTPTarget{Address: addr, ExpectBanner: "mail.example.com"})
	if f.Status != engine.BAD {
		t.Fatalf("want BAD on banner mismatch, got %s: %s", f.Status, f.Message)
	}
}

func TestStartTLSRequiredButNotOffered(t *testing.T) {
	addr := fakeSMTP(t, opts{offerTLS: false})
	f := run(t, engine.SMTPConfig{WarnDays: 30, CritDays: 7},
		engine.SMTPTarget{Address: addr, StartTLS: true})
	if f.Status != engine.BAD || !strings.Contains(f.Message, "STARTTLS") {
		t.Fatalf("want BAD (STARTTLS not offered), got %s: %s", f.Status, f.Message)
	}
}

func TestStartTLSCertExpiring(t *testing.T) {
	cert := genCert(t, 10) // expires within warn window
	addr := fakeSMTP(t, opts{offerTLS: true, startTLS: true, tlsCert: &cert})
	f := run(t, engine.SMTPConfig{WarnDays: 30, CritDays: 7},
		engine.SMTPTarget{Address: addr, StartTLS: true})
	if f.Status != engine.WARN || !strings.Contains(f.Message, "expires in") {
		t.Fatalf("want WARN on expiring cert, got %s: %s", f.Status, f.Message)
	}
}

func TestStartTLSCertExpiredIsBad(t *testing.T) {
	cert := genCert(t, -1) // already expired
	addr := fakeSMTP(t, opts{offerTLS: true, startTLS: true, tlsCert: &cert})
	f := run(t, engine.SMTPConfig{WarnDays: 30, CritDays: 7},
		engine.SMTPTarget{Address: addr, StartTLS: true})
	if f.Status != engine.BAD || !strings.Contains(f.Message, "EXPIRED") {
		t.Fatalf("want BAD on expired cert, got %s: %s", f.Status, f.Message)
	}
}

func TestImplicitTLSOK(t *testing.T) {
	cert := genCert(t, 90)
	addr := fakeSMTP(t, opts{implicitTLS: true, tlsCert: &cert})
	f := run(t, engine.SMTPConfig{WarnDays: 30, CritDays: 7},
		engine.SMTPTarget{Address: addr, TLS: true})
	if f.Status != engine.OK {
		t.Fatalf("want OK over implicit TLS, got %s: %s", f.Status, f.Message)
	}
}

func TestConnectionRefusedIsError(t *testing.T) {
	// Grab a port then close it so the dial is refused.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	f := run(t, engine.SMTPConfig{WarnDays: 30, CritDays: 7}, engine.SMTPTarget{Address: addr})
	if f.Status != engine.ERROR {
		t.Fatalf("want ERROR on refused connection, got %s: %s", f.Status, f.Message)
	}
}

func TestDefaultPort(t *testing.T) {
	if got := withDefaultPort("mail.example.com", false); got != "mail.example.com:25" {
		t.Errorf("plain default = %q, want :25", got)
	}
	if got := withDefaultPort("mail.example.com", true); got != "mail.example.com:465" {
		t.Errorf("tls default = %q, want :465", got)
	}
	if got := withDefaultPort("mail.example.com:587", false); got != "mail.example.com:587" {
		t.Errorf("explicit port must be kept, got %q", got)
	}
}
