package ingest

import (
	"context"
	"io"
	"net"
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// startRTMP starts a fake RTMP server that completes the simple handshake.
func startRTMP(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, _ = io.ReadFull(conn, make([]byte, 1+1536)) // C0+C1
				resp := make([]byte, 1+1536+1536)              // S0+S1+S2
				resp[0] = 0x03
				_, _ = conn.Write(resp)
			}()
		}
	}()
	return ln.Addr().String()
}

// startSRT starts a fake SRT listener that replies with a handshake packet.
func startSRT(t *testing.T) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	go func() {
		buf := make([]byte, 1500)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			_ = n
			_, _ = pc.WriteTo(srtInductionPacket(0x42), addr) // control handshake reply
		}
	}()
	return pc.LocalAddr().String()
}

func run(t *testing.T, targets ...engine.IngestTarget) map[string]engine.Finding {
	t.Helper()
	c := New(engine.IngestConfig{Targets: targets})
	m := map[string]engine.Finding{}
	for _, f := range c.Run(context.Background()) {
		m[f.Target] = f
	}
	return m
}

func TestRTMPReachable(t *testing.T) {
	addr := startRTMP(t)
	got := run(t, engine.IngestTarget{Name: "rtmp", Address: addr, Protocol: "rtmp"})["rtmp"]
	if got.Status != engine.OK {
		t.Errorf("rtmp: want OK, got %s (%s)", got.Status, got.Message)
	}
}

func TestSRTReachable(t *testing.T) {
	addr := startSRT(t)
	got := run(t, engine.IngestTarget{Name: "srt", Address: addr, Protocol: "srt"})["srt"]
	if got.Status != engine.OK {
		t.Errorf("srt: want OK, got %s (%s)", got.Status, got.Message)
	}
}

func TestIngestUnreachable(t *testing.T) {
	got := run(t, engine.IngestTarget{Name: "down", Address: "127.0.0.1:1", Protocol: "rtmp"})["down"]
	if got.Status != engine.ERROR {
		t.Errorf("unreachable: want ERROR, got %s (%s)", got.Status, got.Message)
	}
}

func TestIngestUnknownProtocol(t *testing.T) {
	got := run(t, engine.IngestTarget{Name: "x", Address: "127.0.0.1:1", Protocol: "webrtc"})["x"]
	if got.Status != engine.BAD {
		t.Errorf("unknown protocol: want BAD, got %s (%s)", got.Status, got.Message)
	}
}
