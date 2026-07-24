package grpccheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// startGRPC serves a fake grpc.health.v1.Health/Check over HTTP/2 + TLS.
// If grpcStatus != "0" it returns a trailers-only error; otherwise it replies
// with a HealthCheckResponse carrying servingStatus.
func startGRPC(t *testing.T, servingStatus int, grpcStatus string) string {
	t.Helper()
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/grpc")
		if grpcStatus != "0" {
			w.Header().Set("Grpc-Status", grpcStatus) // trailers-only error
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Trailer", "Grpc-Status")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(grpcFrame([]byte{0x08, byte(servingStatus)})) // field1=status
		w.Header().Set("Grpc-Status", "0")
	})
	srv := httptest.NewUnstartedServer(h)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "https://")
}

func run(t *testing.T, addr, service string) engine.Finding {
	t.Helper()
	c := New(engine.GRPCConfig{Targets: []engine.GRPCTarget{{Address: addr, Service: service, InsecureSkipVerify: true}}})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	f := c.Run(ctx)
	if len(f) != 1 {
		t.Fatalf("want 1 finding, got %d", len(f))
	}
	return f[0]
}

func TestServingIsOK(t *testing.T) {
	if got := run(t, startGRPC(t, statusServing, "0"), ""); got.Status != engine.OK {
		t.Errorf("SERVING: want OK, got %s (%s)", got.Status, got.Message)
	}
}

func TestNotServingIsBad(t *testing.T) {
	if got := run(t, startGRPC(t, statusNotServing, "0"), "mysvc"); got.Status != engine.BAD {
		t.Errorf("NOT_SERVING: want BAD, got %s (%s)", got.Status, got.Message)
	}
}

func TestUnimplementedIsWarn(t *testing.T) {
	if got := run(t, startGRPC(t, 0, "12"), ""); got.Status != engine.WARN {
		t.Errorf("UNIMPLEMENTED: want WARN, got %s (%s)", got.Status, got.Message)
	}
}

func TestServiceUnknownIsBad(t *testing.T) {
	if got := run(t, startGRPC(t, 0, "5"), "ghost"); got.Status != engine.BAD {
		t.Errorf("NOT_FOUND: want BAD, got %s (%s)", got.Status, got.Message)
	}
}

func TestUnreachableIsError(t *testing.T) {
	if got := run(t, "127.0.0.1:1", ""); got.Status != engine.ERROR {
		t.Errorf("unreachable: want ERROR, got %s (%s)", got.Status, got.Message)
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	// Request encoding for a named service, and response decoding.
	req := encodeHealthRequest("svc")
	if len(req) < 2 || req[0] != 0x0A {
		t.Errorf("encode request errato: %v", req)
	}
	if s, ok := decodeHealthResponse(grpcFrame([]byte{0x08, statusServing})); !ok || s != statusServing {
		t.Errorf("decode response: want %d, got %d ok=%v", statusServing, s, ok)
	}
}
