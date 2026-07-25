// Package ingest checks that a streaming ingest endpoint accepts connections —
// the "can the streamer publish?" signal. It speaks just enough of each
// protocol to prove a real server is there: the RTMP handshake (C0/C1 -> S0/S1/S2)
// over TCP, and the SRT induction handshake over UDP. Zero dependencies.
package ingest

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

type Check struct {
	cfg engine.IngestConfig
	now func() time.Time
}

func New(cfg engine.IngestConfig) *Check { return &Check{cfg: cfg, now: time.Now} }

func (c *Check) Name() string { return "ingest" }

func (c *Check) Run(ctx context.Context) []engine.Finding {
	findings := make([]engine.Finding, len(c.cfg.Targets))
	sem := make(chan struct{}, 16)
	done := make(chan int)
	for i, t := range c.cfg.Targets {
		go func(i int, t engine.IngestTarget) {
			sem <- struct{}{}
			findings[i] = c.probe(ctx, t)
			<-sem
			done <- i
		}(i, t)
	}
	for range c.cfg.Targets {
		<-done
	}
	return findings
}

func (c *Check) probe(ctx context.Context, t engine.IngestTarget) engine.Finding {
	label := t.Name
	if label == "" {
		label = t.Address
	}
	f := engine.Finding{Check: c.Name(), Target: label}

	proto := strings.ToLower(t.Protocol)
	if proto == "" {
		proto = "rtmp"
	}

	start := c.now()
	var err error
	switch proto {
	case "rtmp":
		err = rtmpHandshake(ctx, t.Address)
	case "srt":
		err = srtInduction(ctx, t.Address)
	default:
		f.Status, f.Message = engine.BAD, fmt.Sprintf("unknown protocol %q (rtmp|srt)", t.Protocol)
		return f
	}
	if err != nil {
		f.Status, f.Message = engine.ERROR, fmt.Sprintf("%s handshake failed: %v", proto, err)
		return f
	}
	latency := c.now().Sub(start)

	if t.MaxLatencyMS > 0 && latency > time.Duration(t.MaxLatencyMS)*time.Millisecond {
		f.Status, f.Message = engine.WARN, fmt.Sprintf("%s ok but slow: %s (over %dms)", proto, latency.Round(time.Millisecond), t.MaxLatencyMS)
		return f
	}
	f.Status, f.Message = engine.OK, fmt.Sprintf("%s accepts connections (%s)", proto, latency.Round(time.Millisecond))
	return f
}

// rtmpHandshake does the RTMP simple handshake: send C0+C1, read S0+S1+S2 and
// verify the version byte. Enough to prove a real RTMP server, not just an open
// port.
func rtmpHandshake(ctx context.Context, address string) error {
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
	if err != nil {
		return err
	}
	defer conn.Close()
	if dl, ok := ctx.Deadline(); ok {
		conn.SetDeadline(dl)
	} else {
		conn.SetDeadline(time.Now().Add(10 * time.Second))
	}

	c0c1 := make([]byte, 1+1536) // C0 version + C1 (time/zero/random left zero)
	c0c1[0] = 0x03
	if _, err := conn.Write(c0c1); err != nil {
		return err
	}
	resp := make([]byte, 1+1536+1536) // S0 + S1 + S2
	if _, err := io.ReadFull(conn, resp); err != nil {
		return err
	}
	if resp[0] != 0x03 {
		return fmt.Errorf("unexpected RTMP version %d", resp[0])
	}
	return nil
}

// srtInduction sends an SRT induction handshake over UDP and expects a
// handshake control packet back. Best-effort reachability (the full SRT
// handshake has more phases); enough to tell an SRT listener answers.
func srtInduction(ctx context.Context, address string) error {
	conn, err := (&net.Dialer{}).DialContext(ctx, "udp", address)
	if err != nil {
		return err
	}
	defer conn.Close()
	deadline := time.Now().Add(5 * time.Second)
	if dl, ok := ctx.Deadline(); ok {
		deadline = dl
	}
	conn.SetDeadline(deadline)

	if _, err := conn.Write(srtInductionPacket(0x00000001)); err != nil {
		return err
	}
	buf := make([]byte, 1500)
	n, err := conn.Read(buf)
	if err != nil {
		return err
	}
	if !isSRTHandshake(buf[:n]) {
		return fmt.Errorf("not an SRT handshake response (%d bytes)", n)
	}
	return nil
}

// srtInductionPacket builds a UDT/SRT control handshake packet (induction).
func srtInductionPacket(socketID uint32) []byte {
	pkt := make([]byte, 16+48)
	binary.BigEndian.PutUint32(pkt[0:], 0x80000000) // F=1 control, type=0 (handshake)
	// [4:16] additional info, timestamp, dest socket id = 0 for induction
	cif := pkt[16:]
	binary.BigEndian.PutUint32(cif[0:], 4)         // UDT version 4 (triggers SRT v5 negotiation)
	binary.BigEndian.PutUint32(cif[8:], 0)         // initial sequence number
	binary.BigEndian.PutUint32(cif[12:], 1500)     // MTU
	binary.BigEndian.PutUint32(cif[16:], 8192)     // flight flag size
	binary.BigEndian.PutUint32(cif[20:], 1)        // reqtype = URQ_INDUCTION
	binary.BigEndian.PutUint32(cif[24:], socketID) // our socket id
	return pkt
}

// isSRTHandshake reports whether b is a control packet of type handshake.
func isSRTHandshake(b []byte) bool {
	if len(b) < 16 {
		return false
	}
	word0 := binary.BigEndian.Uint32(b[0:4])
	isControl := word0&0x80000000 != 0
	ctrlType := (word0 >> 16) & 0x7fff
	return isControl && ctrlType == 0
}
