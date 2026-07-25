package s3

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

func fakeS3(t *testing.T, gotAuth *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gotAuth != nil {
			*gotAuth = r.Header.Get("Authorization")
		}
		switch r.URL.Path {
		case "/bucket":
			w.WriteHeader(http.StatusOK)
		case "/bucket/fresh.txt":
			w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
			w.WriteHeader(http.StatusOK)
		case "/bucket/old.txt":
			w.Header().Set("Last-Modified", time.Now().Add(-2*time.Hour).UTC().Format(http.TimeFormat))
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func run(t *testing.T, target engine.S3Target) map[string]engine.Finding {
	t.Helper()
	m := map[string]engine.Finding{}
	for _, f := range New(engine.S3Config{Targets: []engine.S3Target{target}}).Run(context.Background()) {
		m[f.Target] = f
	}
	return m
}

func TestBucketAndFreshObject(t *testing.T) {
	srv := fakeS3(t, nil)
	f := run(t, engine.S3Target{Name: "b", Endpoint: srv.URL, Bucket: "bucket", Object: "fresh.txt", MaxAgeWarnSeconds: 60, PathStyle: true})
	if f["b"].Status != engine.OK {
		t.Errorf("bucket: want OK, got %s (%s)", f["b"].Status, f["b"].Message)
	}
	if f["b/fresh.txt"].Status != engine.OK {
		t.Errorf("fresh object: want OK, got %s (%s)", f["b/fresh.txt"].Status, f["b/fresh.txt"].Message)
	}
}

func TestStaleObjectIsWarn(t *testing.T) {
	srv := fakeS3(t, nil)
	f := run(t, engine.S3Target{Name: "b", Endpoint: srv.URL, Bucket: "bucket", Object: "old.txt", MaxAgeWarnSeconds: 60, PathStyle: true})
	if f["b/old.txt"].Status != engine.WARN {
		t.Errorf("stale object: want WARN, got %s (%s)", f["b/old.txt"].Status, f["b/old.txt"].Message)
	}
}

func TestMissingObjectIsBad(t *testing.T) {
	srv := fakeS3(t, nil)
	f := run(t, engine.S3Target{Name: "b", Endpoint: srv.URL, Bucket: "bucket", Object: "missing.txt", PathStyle: true})
	if f["b/missing.txt"].Status != engine.BAD {
		t.Errorf("missing object: want BAD, got %s (%s)", f["b/missing.txt"].Status, f["b/missing.txt"].Message)
	}
}

func TestMissingBucketIsBad(t *testing.T) {
	srv := fakeS3(t, nil)
	f := run(t, engine.S3Target{Name: "b", Endpoint: srv.URL, Bucket: "nobucket", Object: "x", PathStyle: true})
	if f["b"].Status != engine.BAD {
		t.Errorf("missing bucket: want BAD, got %s (%s)", f["b"].Status, f["b"].Message)
	}
	if _, ok := f["b/x"]; ok {
		t.Error("object should not be checked when the bucket is unreachable")
	}
}

func TestUnreachableIsError(t *testing.T) {
	f := run(t, engine.S3Target{Name: "b", Endpoint: "http://127.0.0.1:1", Bucket: "bucket", PathStyle: true})
	if f["b"].Status != engine.ERROR {
		t.Errorf("unreachable: want ERROR, got %s (%s)", f["b"].Status, f["b"].Message)
	}
}

func TestSignsWhenCredsPresent(t *testing.T) {
	t.Setenv("S3_AK", "AKIAEXAMPLE")
	t.Setenv("S3_SK", "secret")
	var auth string
	srv := fakeS3(t, &auth)
	run(t, engine.S3Target{Name: "b", Endpoint: srv.URL, Bucket: "bucket", Region: "us-east-1", PathStyle: true, AccessKeyEnv: "S3_AK", SecretKeyEnv: "S3_SK"})
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 Credential=AKIAEXAMPLE/") || !strings.Contains(auth, "Signature=") {
		t.Errorf("request not signed with SigV4: %q", auth)
	}
}
