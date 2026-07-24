package stream

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// fixedNow is the reference "now" used by the tests.
var fixedNow = time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)

// serveManifests spins an httptest server serving the given path→body map.
func serveManifests(t *testing.T, files map[string]string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	for path, body := range files {
		b := body
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(b))
		})
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func run(t *testing.T, targets ...engine.StreamTarget) []engine.Finding {
	t.Helper()
	c := New(engine.StreamConfig{Targets: targets})
	c.now = func() time.Time { return fixedNow }
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return c.Run(ctx)
}

func byTarget(findings []engine.Finding) map[string]engine.Finding {
	m := map[string]engine.Finding{}
	for _, f := range findings {
		m[f.Target] = f
	}
	return m
}

// liveMedia builds a live media playlist whose live edge is `ageSec` before
// fixedNow: two 6s segments, first PDT set so that first_pdt + 12s = edge.
func liveMedia(ageSec int) string {
	edge := fixedNow.Add(-time.Duration(ageSec) * time.Second)
	firstPDT := edge.Add(-12 * time.Second)
	return "#EXTM3U\n" +
		"#EXT-X-TARGETDURATION:6\n" +
		"#EXT-X-PROGRAM-DATE-TIME:" + firstPDT.Format(time.RFC3339) + "\n" +
		"#EXTINF:6.0,\nseg1.ts\n" +
		"#EXTINF:6.0,\nseg2.ts\n"
}

func TestMasterLadderComplete(t *testing.T) {
	master := "#EXTM3U\n" +
		"#EXT-X-STREAM-INF:BANDWIDTH=800000,RESOLUTION=640x360\nlow.m3u8\n" +
		"#EXT-X-STREAM-INF:BANDWIDTH=2400000,RESOLUTION=1280x720\nmid.m3u8\n" +
		"#EXT-X-STREAM-INF:BANDWIDTH=5000000,RESOLUTION=1920x1080\nhigh.m3u8\n"
	srv := serveManifests(t, map[string]string{"/master.m3u8": master})

	f := byTarget(run(t, engine.StreamTarget{Name: "ch1", URL: srv.URL + "/master.m3u8", MinVariants: 3}))
	if got := f["ch1"]; got.Status != engine.OK {
		t.Errorf("master: want OK, got %s (%s)", got.Status, got.Message)
	}
	if got := f["ch1 [ladder]"]; got.Status != engine.OK {
		t.Errorf("ladder 3/3: want OK, got %s (%s)", got.Status, got.Message)
	}
}

func TestLadderIncompleteIsWarn(t *testing.T) {
	master := "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=800000\nlow.m3u8\n"
	srv := serveManifests(t, map[string]string{"/m.m3u8": master})
	f := byTarget(run(t, engine.StreamTarget{Name: "ch", URL: srv.URL + "/m.m3u8", MinVariants: 3}))
	if got := f["ch [ladder]"]; got.Status != engine.WARN {
		t.Errorf("ladder 1/3: want WARN, got %s (%s)", got.Status, got.Message)
	}
}

func TestLiveEdgeFreshOKWarnBad(t *testing.T) {
	cases := []struct {
		age  int
		want engine.Status
	}{
		{age: 5, want: engine.OK},
		{age: 40, want: engine.WARN},
		{age: 90, want: engine.BAD},
	}
	for _, tc := range cases {
		srv := serveManifests(t, map[string]string{"/live.m3u8": liveMedia(tc.age)})
		f := byTarget(run(t, engine.StreamTarget{
			Name: "live", URL: srv.URL + "/live.m3u8", Live: true,
			MaxAgeWarnSeconds: 30, MaxAgeCritSeconds: 60,
		}))
		if got := f["live [live-edge]"]; got.Status != tc.want {
			t.Errorf("age %ds: want %s, got %s (%s)", tc.age, tc.want, got.Status, got.Message)
		}
	}
}

func TestLiveButVODIsWarn(t *testing.T) {
	vod := "#EXTM3U\n#EXTINF:6.0,\nseg1.ts\n#EXTINF:6.0,\nseg2.ts\n#EXT-X-ENDLIST\n"
	srv := serveManifests(t, map[string]string{"/vod.m3u8": vod})
	f := byTarget(run(t, engine.StreamTarget{Name: "v", URL: srv.URL + "/vod.m3u8", Live: true, MaxAgeWarnSeconds: 30, MaxAgeCritSeconds: 60}))
	if got := f["v [live-edge]"]; got.Status != engine.WARN {
		t.Errorf("live but VOD: want WARN, got %s (%s)", got.Status, got.Message)
	}
}

func TestMasterFetchesVariantForFreshness(t *testing.T) {
	master := "#EXTM3U\n" +
		"#EXT-X-STREAM-INF:BANDWIDTH=800000\nlow.m3u8\n" +
		"#EXT-X-STREAM-INF:BANDWIDTH=5000000\nhigh.m3u8\n"
	srv := serveManifests(t, map[string]string{
		"/master.m3u8": master,
		"/high.m3u8":   liveMedia(90), // top-bandwidth variant is stale
		"/low.m3u8":    liveMedia(1),
	})
	f := byTarget(run(t, engine.StreamTarget{
		Name: "ch", URL: srv.URL + "/master.m3u8", Live: true,
		MaxAgeWarnSeconds: 30, MaxAgeCritSeconds: 60,
	}))
	// It must pick the top-bandwidth variant (high.m3u8, stale) → BAD.
	if got := f["ch [live-edge]"]; got.Status != engine.BAD {
		t.Errorf("freshness from top variant: want BAD, got %s (%s)", got.Status, got.Message)
	}
}

func TestNoPDTNotMeasurable(t *testing.T) {
	live := "#EXTM3U\n#EXT-X-TARGETDURATION:6\n#EXTINF:6.0,\nseg1.ts\n#EXTINF:6.0,\nseg2.ts\n"
	srv := serveManifests(t, map[string]string{"/l.m3u8": live})
	f := byTarget(run(t, engine.StreamTarget{Name: "l", URL: srv.URL + "/l.m3u8", Live: true, MaxAgeWarnSeconds: 30, MaxAgeCritSeconds: 60}))
	if got := f["l [live-edge]"]; got.Status != engine.WARN {
		t.Errorf("without PDT: want WARN not-measurable, got %s (%s)", got.Status, got.Message)
	}
}

func TestDASHDynamicFresh(t *testing.T) {
	pub := fixedNow.Add(-5 * time.Second).Format(time.RFC3339)
	mpd := `<?xml version="1.0"?>
<MPD type="dynamic" publishTime="` + pub + `">
  <Period>
    <AdaptationSet>
      <Representation bandwidth="800000"/>
      <Representation bandwidth="2400000"/>
    </AdaptationSet>
  </Period>
</MPD>`
	srv := serveManifests(t, map[string]string{"/live.mpd": mpd})
	f := byTarget(run(t, engine.StreamTarget{Name: "d", URL: srv.URL + "/live.mpd", Live: true, MinVariants: 2, MaxAgeWarnSeconds: 30, MaxAgeCritSeconds: 60}))
	if got := f["d"]; got.Status != engine.OK {
		t.Errorf("MPD dynamic: want OK, got %s (%s)", got.Status, got.Message)
	}
	if got := f["d [live-edge]"]; got.Status != engine.OK {
		t.Errorf("publishTime fresh: want OK, got %s (%s)", got.Status, got.Message)
	}
	if got := f["d [ladder]"]; got.Status != engine.OK {
		t.Errorf("2 representations: want OK ladder, got %s (%s)", got.Status, got.Message)
	}
}

func TestUnreachableIsError(t *testing.T) {
	f := run(t, engine.StreamTarget{Name: "x", URL: "http://127.0.0.1:1/x.m3u8"})
	if len(f) == 0 || f[0].Status != engine.ERROR {
		t.Errorf("unreachable: want ERROR, got %v", f)
	}
}

func TestInvalidManifestIsBad(t *testing.T) {
	srv := serveManifests(t, map[string]string{"/bad.m3u8": "not a playlist\n"})
	f := run(t, engine.StreamTarget{Name: "b", URL: srv.URL + "/bad.m3u8"})
	if len(f) == 0 || f[0].Status != engine.BAD {
		t.Errorf("invalid manifest: want BAD, got %v", f)
	}
}
