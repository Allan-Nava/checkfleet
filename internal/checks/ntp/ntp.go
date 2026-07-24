// Package ntp implements an NTP clock-offset check using a hand-rolled NTPv3
// client over UDP (no dependency). Clock drift silently breaks TLS validation
// and JWT expiry, so it's worth watching. Read-only (a single SNTP query).
package ntp

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// result is what a query returns: the estimated clock offset and the stratum.
type result struct {
	Offset  time.Duration
	Stratum uint8
}

type Check struct {
	cfg engine.NTPConfig
	// query is injectable for tests; defaults to a live SNTP query.
	query func(ctx context.Context, target string) (result, error)
}

func New(cfg engine.NTPConfig) *Check {
	c := &Check{cfg: cfg}
	c.query = liveQuery
	return c
}

func (c *Check) Name() string { return "ntp" }

func (c *Check) Run(ctx context.Context) []engine.Finding {
	findings := make([]engine.Finding, len(c.cfg.Targets))
	sem := make(chan struct{}, 16)
	done := make(chan int)
	for i, t := range c.cfg.Targets {
		go func(i int, t string) {
			sem <- struct{}{}
			findings[i] = c.probe(ctx, withDefaultPort(t, c.cfg.Port))
			<-sem
			done <- i
		}(i, t)
	}
	for range c.cfg.Targets {
		<-done
	}
	return findings
}

func (c *Check) probe(ctx context.Context, target string) engine.Finding {
	f := engine.Finding{Check: c.Name(), Target: target}
	res, err := c.query(ctx, target)
	if err != nil {
		f.Status, f.Message = engine.ERROR, fmt.Sprintf("query NTP fallita: %v", err)
		return f
	}
	if res.Stratum == 0 || res.Stratum >= 16 {
		f.Status, f.Message = engine.BAD, fmt.Sprintf("server non sincronizzato (stratum %d)", res.Stratum)
		return f
	}
	absMS := res.Offset.Milliseconds()
	if absMS < 0 {
		absMS = -absMS
	}
	msg := fmt.Sprintf("offset %s, stratum %d", res.Offset.Round(time.Millisecond), res.Stratum)
	switch {
	case c.cfg.OffsetCritMS > 0 && absMS >= int64(c.cfg.OffsetCritMS):
		f.Status, f.Message = engine.BAD, msg+fmt.Sprintf(" (oltre %dms)", c.cfg.OffsetCritMS)
	case c.cfg.OffsetWarnMS > 0 && absMS >= int64(c.cfg.OffsetWarnMS):
		f.Status, f.Message = engine.WARN, msg+fmt.Sprintf(" (oltre %dms)", c.cfg.OffsetWarnMS)
	default:
		f.Status, f.Message = engine.OK, msg
	}
	return f
}

// ntpEpochOffset is the seconds between the NTP epoch (1900) and Unix (1970).
const ntpEpochOffset = 2208988800

// liveQuery performs a single SNTP request and computes the clock offset.
func liveQuery(ctx context.Context, target string) (result, error) {
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "udp", target)
	if err != nil {
		return result{}, err
	}
	defer conn.Close()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	} else {
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	}

	req := make([]byte, 48)
	req[0] = 0x1B // LI=0, VN=3, Mode=3 (client)
	t1 := time.Now()
	if _, err := conn.Write(req); err != nil {
		return result{}, err
	}
	resp := make([]byte, 48)
	if _, err := conn.Read(resp); err != nil {
		return result{}, err
	}
	t4 := time.Now()

	stratum := resp[1]
	t2 := ntpToTime(binary.BigEndian.Uint64(resp[32:40])) // server receive
	t3 := ntpToTime(binary.BigEndian.Uint64(resp[40:48])) // server transmit
	// offset = ((T2 - T1) + (T3 - T4)) / 2
	offset := (t2.Sub(t1) + t3.Sub(t4)) / 2
	return result{Offset: offset, Stratum: stratum}, nil
}

func ntpToTime(ts uint64) time.Time {
	secs := int64(ts>>32) - ntpEpochOffset
	frac := ts & 0xFFFFFFFF
	nsec := int64(float64(frac) / (1 << 32) * 1e9)
	return time.Unix(secs, nsec)
}

func withDefaultPort(target string, port int) string {
	if strings.Contains(target, ":") {
		return target
	}
	return fmt.Sprintf("%s:%d", target, port)
}
