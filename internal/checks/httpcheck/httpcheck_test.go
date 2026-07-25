package httpcheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

func run(t *testing.T, targets ...engine.HTTPTarget) []engine.Finding {
	t.Helper()
	check := New(engine.HTTPConfig{Targets: targets})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return check.Run(ctx)
}

func TestStatusLatencyAndBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			_, _ = w.Write([]byte("hello checkfleet"))
		case "/slow":
			time.Sleep(150 * time.Millisecond)
			_, _ = w.Write([]byte("slow"))
		case "/teapot":
			w.WriteHeader(http.StatusTeapot)
		}
	}))
	defer srv.Close()

	findings := run(t,
		engine.HTTPTarget{URL: srv.URL + "/ok", ExpectStatus: 200, ExpectBody: "checkfleet"},
		engine.HTTPTarget{URL: srv.URL + "/ok", ExpectStatus: 200, ExpectBody: "missing-string"},
		engine.HTTPTarget{URL: srv.URL + "/slow", ExpectStatus: 200, MaxLatencyMS: 50},
		engine.HTTPTarget{URL: srv.URL + "/teapot", ExpectStatus: 200},
		engine.HTTPTarget{URL: "http://127.0.0.1:1/down", ExpectStatus: 200},
	)

	want := []engine.Status{engine.OK, engine.BAD, engine.WARN, engine.BAD, engine.ERROR}
	for i, w := range want {
		if findings[i].Status != w {
			t.Errorf("target %d: want %s, got %s (%s)", i, w, findings[i].Status, findings[i].Message)
		}
	}
	// The OK finding carries the latency metric (CF-91); the network ERROR does not.
	if findings[0].Value == nil || findings[0].Unit != "ms" {
		t.Errorf("OK finding: want a ms latency Value, got value=%v unit=%q", findings[0].Value, findings[0].Unit)
	}
	if findings[4].Value != nil {
		t.Errorf("ERROR finding: want no Value (nothing measured), got %v", *findings[4].Value)
	}
}
