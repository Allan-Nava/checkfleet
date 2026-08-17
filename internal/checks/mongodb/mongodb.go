// Package mongodb implements a read-only health check for MongoDB: replica-set
// status (primary present, member health, replication lag) via replSetGetStatus
// and connection saturation via serverStatus. It never writes.
//
// Database access is abstracted behind the collector interface so the finding
// logic is unit-tested with a fake; the mongo-driver-backed collector lives in
// mongo.go.
package mongodb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// metrics is the read-only snapshot a collector returns.
type metrics struct {
	Version     string
	Standalone  bool // not running as a replica set
	Connections int64
	Available   int64
	ReplSet     string
	Members     []member // from replSetGetStatus (empty when standalone)
}

type member struct {
	Name     string    // host:port
	Health   float64   // 1 healthy, 0 down
	StateStr string    // PRIMARY, SECONDARY, ARBITER, ...
	Optime   time.Time // last applied op
}

// collector gathers metrics from one target. Close releases the connection.
type collector interface {
	Collect(ctx context.Context) (metrics, error)
	Close(ctx context.Context)
}

type Check struct {
	cfg engine.MongoDBConfig
	// connect is injectable for tests; defaults to the mongo-driver collector.
	connect func(ctx context.Context, t engine.MongoDBTarget) (collector, error)
}

func New(cfg engine.MongoDBConfig) *Check {
	return &Check{cfg: cfg, connect: mongoConnect}
}

func (c *Check) Name() string { return "mongodb" }

func (c *Check) Run(ctx context.Context) []engine.Finding {
	perTarget := make([][]engine.Finding, len(c.cfg.Targets))
	sem := make(chan struct{}, 8)
	done := make(chan int)
	for i, t := range c.cfg.Targets {
		go func(i int, t engine.MongoDBTarget) {
			sem <- struct{}{}
			perTarget[i] = c.probe(ctx, t)
			<-sem
			done <- i
		}(i, t)
	}
	for range c.cfg.Targets {
		<-done
	}
	var findings []engine.Finding
	for _, fs := range perTarget {
		findings = append(findings, fs...)
	}
	return findings
}

func (c *Check) probe(ctx context.Context, t engine.MongoDBTarget) []engine.Finding {
	label := t.Name
	if label == "" {
		label = hostOf(t.URI)
	}

	col, err := c.connect(ctx, t)
	if err != nil {
		return []engine.Finding{{Check: c.Name(), Target: label, Status: engine.ERROR,
			Message: fmt.Sprintf("connection failed: %v", engine.Redact(err.Error()))}}
	}
	defer col.Close(ctx)

	m, err := col.Collect(ctx)
	if err != nil {
		return []engine.Finding{{Check: c.Name(), Target: label, Status: engine.ERROR,
			Message: fmt.Sprintf("status query failed: %v", engine.Redact(err.Error()))}}
	}

	var findings []engine.Finding

	// Reachability.
	kind := "standalone"
	if !m.Standalone {
		kind = "replica set " + m.ReplSet
	}
	findings = append(findings, engine.Finding{Check: c.Name(), Target: label, Status: engine.OK,
		Message: fmt.Sprintf("reachable, MongoDB %s (%s)", m.Version, kind)})

	// Connection saturation.
	findings = append(findings, c.connectionFinding(label, m))

	// Replica-set health (skipped for standalone).
	if !m.Standalone {
		findings = append(findings, c.replicaFindings(label, m)...)
	}
	return findings
}

func (c *Check) connectionFinding(label string, m metrics) engine.Finding {
	f := engine.Finding{Check: c.Name(), Target: label + "/connections"}
	total := m.Connections + m.Available
	if total <= 0 {
		f.Status, f.Message = engine.OK, fmt.Sprintf("%d connections", m.Connections)
		return f
	}
	pct := int(m.Connections * 100 / total)
	if c.cfg.ConnWarnPct > 0 && pct >= c.cfg.ConnWarnPct {
		f.Status = engine.WARN
		f.Message = fmt.Sprintf("%d/%d connections in use (%d%%, over %d%%)", m.Connections, total, pct, c.cfg.ConnWarnPct)
		return f
	}
	f.Status, f.Message = engine.OK, fmt.Sprintf("%d/%d connections in use (%d%%)", m.Connections, total, pct)
	return f
}

func (c *Check) replicaFindings(label string, m metrics) []engine.Finding {
	var findings []engine.Finding

	// Locate the primary and its optime (the lag reference).
	var primaryOptime time.Time
	primaryPresent := false
	for _, mem := range m.Members {
		if mem.StateStr == "PRIMARY" && mem.Health == 1 {
			primaryPresent = true
			primaryOptime = mem.Optime
		}
	}
	if !primaryPresent {
		findings = append(findings, engine.Finding{Check: c.Name(), Target: label + "/primary",
			Status: engine.BAD, Message: "no healthy PRIMARY in the replica set"})
	}

	for _, mem := range m.Members {
		f := engine.Finding{Check: c.Name(), Target: label + "/" + mem.Name}
		switch {
		case mem.Health != 1:
			f.Status, f.Message = engine.BAD, fmt.Sprintf("%s unreachable (health %.0f)", mem.StateStr, mem.Health)
		case mem.StateStr == "SECONDARY" && primaryPresent:
			lag := primaryOptime.Sub(mem.Optime)
			if lag < 0 {
				lag = 0
			}
			secs := int(lag.Seconds())
			switch {
			case c.cfg.LagCritSeconds > 0 && secs >= c.cfg.LagCritSeconds:
				f.Status, f.Message = engine.BAD, fmt.Sprintf("SECONDARY lag %ds (over %ds)", secs, c.cfg.LagCritSeconds)
			case c.cfg.LagWarnSeconds > 0 && secs >= c.cfg.LagWarnSeconds:
				f.Status, f.Message = engine.WARN, fmt.Sprintf("SECONDARY lag %ds (over %ds)", secs, c.cfg.LagWarnSeconds)
			default:
				f.Status, f.Message = engine.OK, fmt.Sprintf("SECONDARY, lag %ds", secs)
			}
		default:
			f.Status, f.Message = engine.OK, mem.StateStr
		}
		findings = append(findings, f)
	}
	return findings
}

// hostOf extracts a host:port label from a mongodb URI, best-effort.
func hostOf(uri string) string {
	s := strings.TrimPrefix(uri, "mongodb+srv://")
	s = strings.TrimPrefix(s, "mongodb://")
	if i := strings.IndexByte(s, '@'); i >= 0 { // strip any embedded credentials
		s = s[i+1:]
	}
	if i := strings.IndexAny(s, "/?"); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return uri
	}
	return s
}
