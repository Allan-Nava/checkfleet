// Package mysql implements a read-only health check for MySQL/MariaDB: server
// reachable, connection saturation (Threads_connected vs max_connections),
// read-only role, and — on a replica — replication health and lag. It never
// writes.
//
// Database access is abstracted behind the collector interface so the finding
// logic is unit-tested with a fake; the go-sql-driver-backed collector lives in
// driver.go.
package mysql

import (
	"context"
	"fmt"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// metrics is the read-only snapshot a collector returns.
type metrics struct {
	Version        string
	ReadOnly       bool
	Connections    int
	MaxConnections int
	Replica        *replicaStatus // nil when the server is not a replica
}

// replicaStatus mirrors the relevant SHOW REPLICA/SLAVE STATUS fields.
type replicaStatus struct {
	IORunning     bool
	SQLRunning    bool
	SecondsBehind int64 // valid only when Replicating is true
	Replicating   bool  // Seconds_Behind_Source was non-NULL
}

// collector gathers metrics from one target. Close releases the connection.
type collector interface {
	Collect(ctx context.Context) (metrics, error)
	Close()
}

type Check struct {
	cfg engine.MySQLConfig
	// connect is injectable for tests; defaults to the driver-backed collector.
	connect func(ctx context.Context, t engine.MySQLTarget) (collector, error)
}

func New(cfg engine.MySQLConfig) *Check {
	return &Check{cfg: cfg, connect: driverConnect}
}

func (c *Check) Name() string { return "mysql" }

func (c *Check) Run(ctx context.Context) []engine.Finding {
	perTarget := make([][]engine.Finding, len(c.cfg.Targets))
	sem := make(chan struct{}, 8)
	done := make(chan int)
	for i, t := range c.cfg.Targets {
		go func(i int, t engine.MySQLTarget) {
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

func (c *Check) probe(ctx context.Context, t engine.MySQLTarget) []engine.Finding {
	label := t.Name
	if label == "" {
		label = "mysql"
	}

	col, err := c.connect(ctx, t)
	if err != nil {
		return []engine.Finding{{Check: c.Name(), Target: label, Status: engine.ERROR,
			Message: fmt.Sprintf("connection failed: %v", err)}}
	}
	defer col.Close()

	m, err := col.Collect(ctx)
	if err != nil {
		return []engine.Finding{{Check: c.Name(), Target: label, Status: engine.ERROR,
			Message: fmt.Sprintf("status query failed: %v", err)}}
	}

	role := "read-write"
	if m.ReadOnly {
		role = "read-only"
	}
	findings := []engine.Finding{{Check: c.Name(), Target: label, Status: engine.OK,
		Message: fmt.Sprintf("reachable, MySQL %s (%s)", m.Version, role)}}

	findings = append(findings, c.connectionFinding(label, m))
	if m.Replica != nil {
		findings = append(findings, c.replicaFinding(label, m))
	}
	return findings
}

func (c *Check) connectionFinding(label string, m metrics) engine.Finding {
	f := engine.Finding{Check: c.Name(), Target: label + "/connections"}
	if m.MaxConnections <= 0 {
		f.Status, f.Message = engine.OK, fmt.Sprintf("%d connections", m.Connections)
		return f
	}
	pct := m.Connections * 100 / m.MaxConnections
	if c.cfg.ConnWarnPct > 0 && pct >= c.cfg.ConnWarnPct {
		f.Status = engine.WARN
		f.Message = fmt.Sprintf("%d/%d connections in use (%d%%, over %d%%)", m.Connections, m.MaxConnections, pct, c.cfg.ConnWarnPct)
		return f
	}
	f.Status, f.Message = engine.OK, fmt.Sprintf("%d/%d connections in use (%d%%)", m.Connections, m.MaxConnections, pct)
	return f
}

func (c *Check) replicaFinding(label string, m metrics) engine.Finding {
	f := engine.Finding{Check: c.Name(), Target: label + "/replication"}
	r := m.Replica
	if r.Replicating {
		f.Value, f.Unit = engine.Num(float64(r.SecondsBehind)), "s" // replica lag
	}
	switch {
	case !r.IORunning || !r.SQLRunning:
		f.Status = engine.BAD
		f.Message = fmt.Sprintf("replication stopped (IO=%v, SQL=%v)", r.IORunning, r.SQLRunning)
	case !r.Replicating:
		f.Status, f.Message = engine.BAD, "replica not replicating (Seconds_Behind is NULL)"
	case c.cfg.LagCritSeconds > 0 && r.SecondsBehind >= int64(c.cfg.LagCritSeconds):
		f.Status = engine.BAD
		f.Message = fmt.Sprintf("replica %ds behind (over %ds)", r.SecondsBehind, c.cfg.LagCritSeconds)
	case c.cfg.LagWarnSeconds > 0 && r.SecondsBehind >= int64(c.cfg.LagWarnSeconds):
		f.Status = engine.WARN
		f.Message = fmt.Sprintf("replica %ds behind (over %ds)", r.SecondsBehind, c.cfg.LagWarnSeconds)
	default:
		f.Status, f.Message = engine.OK, fmt.Sprintf("replica healthy, %ds behind", r.SecondsBehind)
	}
	return f
}
