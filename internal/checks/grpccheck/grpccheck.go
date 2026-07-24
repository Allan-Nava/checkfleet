// Package grpccheck implements the gRPC Health Checking Protocol
// (grpc.health.v1.Health/Check) over HTTP/2 + TLS, with the request/response
// protobuf messages encoded by hand — no gRPC library dependency. Plaintext
// h2c is not supported (it would need golang.org/x/net); TLS endpoints only.
package grpccheck

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// serving statuses from grpc.health.v1.HealthCheckResponse.ServingStatus.
const (
	statusUnknown        = 0
	statusServing        = 1
	statusNotServing     = 2
	statusServiceUnknown = 3
)

type Check struct {
	cfg    engine.GRPCConfig
	client func(insecure bool, host string) *http.Client
}

func New(cfg engine.GRPCConfig) *Check {
	return &Check{cfg: cfg, client: defaultClient}
}

func (c *Check) Name() string { return "grpc" }

func (c *Check) Run(ctx context.Context) []engine.Finding {
	findings := make([]engine.Finding, len(c.cfg.Targets))
	sem := make(chan struct{}, 16)
	done := make(chan int)
	for i, t := range c.cfg.Targets {
		go func(i int, t engine.GRPCTarget) {
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

func (c *Check) probe(ctx context.Context, t engine.GRPCTarget) engine.Finding {
	label := t.Name
	if label == "" {
		label = t.Address
		if t.Service != "" {
			label += "/" + t.Service
		}
	}
	f := engine.Finding{Check: c.Name(), Target: label}

	body := grpcFrame(encodeHealthRequest(t.Service))
	url := "https://" + t.Address + "/grpc.health.v1.Health/Check"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		f.Status, f.Message = engine.ERROR, err.Error()
		return f
	}
	req.Header.Set("Content-Type", "application/grpc+proto")
	req.Header.Set("TE", "trailers")

	resp, err := c.client(t.InsecureSkipVerify, hostOf(t.Address)).Do(req)
	if err != nil {
		f.Status, f.Message = engine.ERROR, fmt.Sprintf("connessione gRPC fallita: %v", err)
		return f
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	// grpc-status can arrive in trailers or (trailers-only responses) headers.
	if gs := firstNonEmpty(resp.Trailer.Get("Grpc-Status"), resp.Header.Get("Grpc-Status")); gs != "" && gs != "0" {
		switch gs {
		case "12": // UNIMPLEMENTED
			f.Status, f.Message = engine.WARN, "health service non implementato (grpc-status 12)"
		case "5": // NOT_FOUND
			f.Status, f.Message = engine.BAD, "servizio sconosciuto (grpc-status 5)"
		default:
			f.Status, f.Message = engine.ERROR, "grpc-status "+gs
		}
		return f
	}

	status, ok := decodeHealthResponse(payload)
	if !ok {
		f.Status, f.Message = engine.ERROR, "risposta gRPC non decodificabile"
		return f
	}
	switch status {
	case statusServing:
		f.Status, f.Message = engine.OK, "SERVING"
	case statusNotServing:
		f.Status, f.Message = engine.BAD, "NOT_SERVING"
	case statusServiceUnknown:
		f.Status, f.Message = engine.BAD, "SERVICE_UNKNOWN"
	default:
		f.Status, f.Message = engine.WARN, "UNKNOWN"
	}
	return f
}

func defaultClient(insecure bool, host string) *http.Client {
	return &http.Client{Transport: &http.Transport{
		ForceAttemptHTTP2: true,
		TLSClientConfig:   &tls.Config{ServerName: host, InsecureSkipVerify: insecure},
	}}
}

// ---------- protobuf / gRPC framing (hand-rolled) ----------

// encodeHealthRequest encodes HealthCheckRequest{ service = 1 (string) }.
func encodeHealthRequest(service string) []byte {
	if service == "" {
		return nil
	}
	var b []byte
	b = append(b, 0x0A) // field 1, wire type 2 (length-delimited)
	b = appendVarint(b, uint64(len(service)))
	return append(b, service...)
}

// decodeHealthResponse reads HealthCheckResponse{ status = 1 (enum/varint) }
// from a gRPC-framed message.
func decodeHealthResponse(frame []byte) (int, bool) {
	if len(frame) < 5 {
		return 0, false
	}
	msgLen := binary.BigEndian.Uint32(frame[1:5])
	msg := frame[5:]
	if int(msgLen) > len(msg) {
		return 0, false
	}
	msg = msg[:msgLen]
	for i := 0; i < len(msg); {
		tag := msg[i]
		i++
		if tag == 0x08 { // field 1, varint
			v, n := readVarint(msg[i:])
			if n == 0 {
				return 0, false
			}
			return int(v), true
		}
		return 0, false // unexpected field
	}
	// Empty message → default status UNKNOWN(0), which is valid.
	return statusUnknown, true
}

func grpcFrame(msg []byte) []byte {
	frame := make([]byte, 5+len(msg))
	frame[0] = 0 // uncompressed
	binary.BigEndian.PutUint32(frame[1:5], uint32(len(msg)))
	copy(frame[5:], msg)
	return frame
}

func appendVarint(b []byte, v uint64) []byte {
	for v >= 0x80 {
		b = append(b, byte(v)|0x80)
		v >>= 7
	}
	return append(b, byte(v))
}

func readVarint(b []byte) (uint64, int) {
	var v uint64
	var shift uint
	for i, c := range b {
		v |= uint64(c&0x7F) << shift
		if c < 0x80 {
			return v, i + 1
		}
		shift += 7
	}
	return 0, 0
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func hostOf(addr string) string {
	if h, _, err := net.SplitHostPort(addr); err == nil {
		return h
	}
	return addr
}
