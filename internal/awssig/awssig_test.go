package awssig

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSignSetsHeadersDeterministically(t *testing.T) {
	mk := func() *http.Request {
		r, _ := http.NewRequest(http.MethodPost, "https://sns.eu-west-1.amazonaws.com/", strings.NewReader("Action=Publish"))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return r
	}
	when := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)

	r1 := mk()
	Sign(r1, []byte("Action=Publish"), "AKID", "secret", "eu-west-1", "sns", when)
	auth := r1.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 Credential=AKID/20260725/eu-west-1/sns/aws4_request") {
		t.Fatalf("credential scope wrong: %s", auth)
	}
	if !strings.Contains(auth, "SignedHeaders=content-type;host;x-amz-date") {
		t.Errorf("signed headers wrong: %s", auth)
	}
	if r1.Header.Get("X-Amz-Date") != "20260725T120000Z" {
		t.Errorf("amz date wrong: %s", r1.Header.Get("X-Amz-Date"))
	}

	// Same inputs → same signature (deterministic).
	r2 := mk()
	Sign(r2, []byte("Action=Publish"), "AKID", "secret", "eu-west-1", "sns", when)
	if r1.Header.Get("Authorization") != r2.Header.Get("Authorization") {
		t.Error("signature not deterministic for identical inputs")
	}

	// Different body → different signature.
	r3 := mk()
	Sign(r3, []byte("Action=Other"), "AKID", "secret", "eu-west-1", "sns", when)
	if r1.Header.Get("Authorization") == r3.Header.Get("Authorization") {
		t.Error("signature should change with the body")
	}
}
