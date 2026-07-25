// Package cassandra implements a reachability check for Cassandra/ScyllaDB by
// speaking the CQL native protocol handshake (OPTIONS → SUPPORTED, STARTUP →
// READY/AUTHENTICATE). It confirms the node accepts CQL connections and measures
// handshake latency, without a driver dependency and without authenticating.
package cassandra

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

// CQL native protocol v4 opcodes.
const (
	protoRequest  = 0x04
	protoResponse = 0x84
	opError       = 0x00
	opStartup     = 0x01
	opReady       = 0x02
	opAuthenticate = 0x03
	opOptions     = 0x05
	opSupported   = 0x06
)

type Check struct {
	cfg engine.CassandraConfig
	now func() time.Time
}

func New(cfg engine.CassandraConfig) *Check { return &Check{cfg: cfg, now: time.Now} }

func (c *Check) Name() string { return "cassandra" }

func (c *Check) Run(ctx context.Context) []engine.Finding {
	findings := make([]engine.Finding, len(c.cfg.Targets))
	sem := make(chan struct{}, 16)
	done := make(chan int)
	for i, t := range c.cfg.Targets {
		go func(i int, t engine.CassandraTarget) {
			sem <- struct{}{}
			findings[i] = c.probe(ctx, t)
			<-sem
			done <- i
		}(i, t)
	}
	for range c.cfg.Targets {
		<-done
	}
	if f, ok := c.clusterFinding(findings); ok {
		findings = append(findings, f)
	}
	return findings
}

// clusterFinding rolls the per-node results up into cluster state: how many
// nodes accept CQL out of those configured.
//
// This is deliberately derived from our own probes rather than from
// system.peers: querying the cluster's view of its members needs a QUERY frame
// on an authenticated session, and this module speaks the handshake only
// (zero-dep, no credentials). What it gives up is the state of nodes that are
// not in the config; what it gains is that it works on secured clusters.
// Same shape as the etcd module's expect_members.
func (c *Check) clusterFinding(nodes []engine.Finding) (engine.Finding, bool) {
	// With a single node and no expectation the rollup only repeats it.
	if len(nodes) == 0 || (len(nodes) == 1 && c.cfg.ExpectNodes == 0) {
		return engine.Finding{}, false
	}
	up := 0
	for _, f := range nodes {
		if acceptsCQL(f.Status) {
			up++
		}
	}
	want := c.cfg.ExpectNodes
	if want == 0 {
		want = len(nodes)
	}
	f := engine.Finding{Check: c.Name(), Target: "cluster",
		Value: engine.Num(float64(up)), Unit: "nodes"}
	switch {
	case up < want:
		f.Status = engine.BAD
		f.Message = fmt.Sprintf("%d/%d nodes accept CQL, expected %d", up, len(nodes), want)
	case up < len(nodes):
		// Expectation met, but a configured node is still down.
		f.Status = engine.WARN
		f.Message = fmt.Sprintf("%d/%d nodes accept CQL (expected %d, met)", up, len(nodes), want)
	default:
		f.Status = engine.OK
		f.Message = fmt.Sprintf("%d/%d nodes accept CQL", up, len(nodes))
	}
	return f, true
}

// acceptsCQL reports whether a node finding means the node took the handshake:
// WARN is a slow but working node, BAD/ERROR are not usable.
func acceptsCQL(s engine.Status) bool { return s == engine.OK || s == engine.WARN }

func (c *Check) probe(ctx context.Context, t engine.CassandraTarget) engine.Finding {
	addr := withDefaultPort(t.Address)
	label := t.Name
	if label == "" {
		label = addr
	}
	f := engine.Finding{Check: c.Name(), Target: label}

	start := c.now()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		f.Status, f.Message = engine.ERROR, fmt.Sprintf("connection failed: %v", err)
		return f
	}
	defer conn.Close()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	} else {
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	}

	// OPTIONS → SUPPORTED (negotiates without auth; also lists CQL_VERSION).
	if err := writeFrame(conn, opOptions, nil); err != nil {
		f.Status, f.Message = engine.ERROR, fmt.Sprintf("OPTIONS write failed: %v", err)
		return f
	}
	op, body, err := readFrame(conn)
	if err != nil {
		f.Status, f.Message = engine.ERROR, fmt.Sprintf("OPTIONS read failed: %v", err)
		return f
	}
	if op != opSupported {
		f.Status, f.Message = engine.BAD, fmt.Sprintf("unexpected OPTIONS reply (opcode 0x%02x)", op)
		return f
	}
	cqlVersion := firstCQLVersion(body)

	// STARTUP → READY or AUTHENTICATE (both mean the node is accepting CQL).
	if err := writeFrame(conn, opStartup, stringMap(map[string]string{"CQL_VERSION": cqlVersion})); err != nil {
		f.Status, f.Message = engine.ERROR, fmt.Sprintf("STARTUP write failed: %v", err)
		return f
	}
	op, body, err = readFrame(conn)
	if err != nil {
		f.Status, f.Message = engine.ERROR, fmt.Sprintf("STARTUP read failed: %v", err)
		return f
	}
	latency := c.now().Sub(start)

	switch op {
	case opReady, opAuthenticate:
		note := ""
		if op == opAuthenticate {
			note = " (auth required)"
		}
		// Handshake latency is the module's numeric metric (CF-91/CF-97).
		f.Value, f.Unit = engine.Num(float64(latency.Microseconds())/1000), "ms"
		if t.MaxLatencyMS > 0 && latency > time.Duration(t.MaxLatencyMS)*time.Millisecond {
			f.Status = engine.WARN
			f.Message = fmt.Sprintf("CQL up%s in %s (over %dms)", note, latency.Round(time.Millisecond), t.MaxLatencyMS)
			return f
		}
		f.Status = engine.OK
		f.Message = fmt.Sprintf("CQL %s up%s (%s)", cqlVersion, note, latency.Round(time.Millisecond))
	case opError:
		f.Status, f.Message = engine.BAD, "STARTUP rejected: "+errorMessage(body)
	default:
		f.Status, f.Message = engine.BAD, fmt.Sprintf("unexpected STARTUP reply (opcode 0x%02x)", op)
	}
	return f
}

func writeFrame(conn net.Conn, opcode byte, body []byte) error {
	hdr := make([]byte, 9+len(body))
	hdr[0] = protoRequest
	hdr[1] = 0x00 // flags
	binary.BigEndian.PutUint16(hdr[2:4], 1) // stream id
	hdr[4] = opcode
	binary.BigEndian.PutUint32(hdr[5:9], uint32(len(body)))
	copy(hdr[9:], body)
	_, err := conn.Write(hdr)
	return err
}

func readFrame(conn net.Conn) (byte, []byte, error) {
	hdr := make([]byte, 9)
	if _, err := io.ReadFull(conn, hdr); err != nil {
		return 0, nil, err
	}
	if hdr[0] != protoResponse {
		return 0, nil, fmt.Errorf("not a CQL response frame (version 0x%02x)", hdr[0])
	}
	n := binary.BigEndian.Uint32(hdr[5:9])
	if n > 1<<20 {
		return 0, nil, fmt.Errorf("frame too large (%d bytes)", n)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(conn, body); err != nil {
		return 0, nil, err
	}
	return hdr[4], body, nil
}

// stringMap encodes a CQL [string map].
func stringMap(m map[string]string) []byte {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, uint16(len(m)))
	for k, v := range m {
		buf = appendString(buf, k)
		buf = appendString(buf, v)
	}
	return buf
}

func appendString(buf []byte, s string) []byte {
	var l [2]byte
	binary.BigEndian.PutUint16(l[:], uint16(len(s)))
	buf = append(buf, l[:]...)
	return append(buf, s...)
}

// firstCQLVersion reads the SUPPORTED [string multimap] and returns the first
// advertised CQL_VERSION, falling back to a widely-supported default.
func firstCQLVersion(body []byte) string {
	r := &reader{b: body}
	n, ok := r.u16()
	if !ok {
		return "3.0.0"
	}
	for i := 0; i < n; i++ {
		key, ok := r.str()
		if !ok {
			break
		}
		vals, ok := r.strList()
		if !ok {
			break
		}
		if strings.EqualFold(key, "CQL_VERSION") && len(vals) > 0 {
			return vals[0]
		}
	}
	return "3.0.0"
}

// errorMessage extracts the message from an ERROR frame ([int code][string msg]).
func errorMessage(body []byte) string {
	if len(body) < 4 {
		return "unknown error"
	}
	r := &reader{b: body[4:]}
	if s, ok := r.str(); ok {
		return s
	}
	return "unknown error"
}

// reader is a minimal big-endian cursor over a CQL frame body.
type reader struct {
	b []byte
	i int
}

func (r *reader) u16() (int, bool) {
	if r.i+2 > len(r.b) {
		return 0, false
	}
	v := int(binary.BigEndian.Uint16(r.b[r.i:]))
	r.i += 2
	return v, true
}

func (r *reader) str() (string, bool) {
	n, ok := r.u16()
	if !ok || r.i+n > len(r.b) {
		return "", false
	}
	s := string(r.b[r.i : r.i+n])
	r.i += n
	return s, true
}

func (r *reader) strList() ([]string, bool) {
	n, ok := r.u16()
	if !ok {
		return nil, false
	}
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		s, ok := r.str()
		if !ok {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

func withDefaultPort(addr string) string {
	if _, _, err := net.SplitHostPort(addr); err == nil {
		return addr
	}
	return addr + ":9042"
}
