package stream

import "testing"

// FuzzParseM3U8 feeds arbitrary manifest bodies (and base URLs) to the HLS
// parser. The manifest comes off the network from an untrusted origin, so the
// parser must never panic — it may only return a playlist or an error.
func FuzzParseM3U8(f *testing.F) {
	seeds := []string{
		"",
		"#EXTM3U\n",
		"#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1200000\nvariant.m3u8\n",
		"#EXTM3U\n#EXTINF:6.0,\nseg1.ts\n#EXTINF:6.0,\nseg2.ts\n#EXT-X-ENDLIST\n",
		"#EXTM3U\n#EXT-X-PROGRAM-DATE-TIME:2020-01-01T00:00:00Z\n#EXTINF:6,\nseg.ts\n",
		"#EXT-X-STREAM-INF:BANDWIDTH=\n#EXTINF:abc,\n",
	}
	for _, s := range seeds {
		f.Add(s, "https://example.com/live/master.m3u8")
	}
	f.Fuzz(func(t *testing.T, body, base string) {
		_, _ = parseM3U8(body, base)
	})
}
