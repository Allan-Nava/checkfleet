// Package postgres implements a read-only health check for PostgreSQL: it
// connects to each target and evaluates transaction-id wraparound risk,
// connection saturation, inactive replication slots retaining WAL, and — on a
// primary — replica lag. All queries are read-only; the check never runs DDL
// or writes.
//
// Database access is abstracted behind the collector interface so the
// finding logic is unit-tested with a fake; the pgx-backed collector lives in
// pgx.go.
package postgres

import (
	"context"
	"fmt"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// metrics is the read-only snapshot a collector returns.
type metrics struct {
	InRecovery     bool
	WraparoundAge  int64 // max(age(datfrozenxid)) across databases
	Connections    int
	MaxConnections int
	Replicas       []replica // from pg_stat_replication (primary only)
	InactiveSlots  []slot    // inactive replication slots retaining WAL
}

type replica struct {
	Client   string
	State    string
	LagBytes int64
}

type slot struct {
	Name          string
	RetainedBytes int64
}

// collector gathers metrics from one target. Close releases the connection.
type collector interface {
	Collect(ctx context.Context) (metrics, error)
	Close(ctx context.Context)
}

type Check struct {
	cfg engine.PostgresConfig
	// connect is injectable for tests; defaults to the pgx-backed collector.
	connect func(ctx context.Context, t engine.PostgresTarget) (collector, error)
}

func New(cfg engine.PostgresConfig) *Check {
	return &Check{cfg: cfg, connect: pgxConnect}
}

func (c *Check) Name() string { return "postgres" }

func (c *Check) Run(ctx context.Context) []engine.Finding {
	perTarget := make([][]engine.Finding, len(c.cfg.Targets))
	sem := make(chan struct{}, 8)
	done := make(chan int)
	for i, t := range c.cfg.Targets {
		go func(i int, t engine.PostgresTarget) {
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

func (c *Check) probe(ctx context.Context, t engine.PostgresTarget) []engine.Finding {
	label := t.Name
	if label == "" {
		label = t.DSN
	}
	coll, err := c.connect(ctx, t)
	if err != nil {
		return []engine.Finding{{Check: c.Name(), Target: label, Status: engine.ERROR, Message: fmt.Sprintf("connessione fallita: %v", err)}}
	}
	defer coll.Close(ctx)

	m, err := coll.Collect(ctx)
	if err != nil {
		return []engine.Finding{{Check: c.Name(), Target: label, Status: engine.ERROR, Message: fmt.Sprintf("query fallita: %v", err)}}
	}
	return c.evaluate(label, m)
}

// evaluate turns a metrics snapshot into findings.
func (c *Check) evaluate(label string, m metrics) []engine.Finding {
	role := "primary"
	if m.InRecovery {
		role = "replica"
	}
	findings := []engine.Finding{{
		Check: c.Name(), Target: label, Status: engine.OK,
		Message: fmt.Sprintf("raggiungibile (%s)", role),
	}}

	findings = append(findings, c.wraparoundFinding(label, m.WraparoundAge))
	findings = append(findings, c.connectionsFinding(label, m))
	findings = append(findings, c.slotFindings(label, m.InactiveSlots)...)
	if !m.InRecovery {
		findings = append(findings, c.replicaFindings(label, m.Replicas)...)
	}
	return findings
}

func (c *Check) wraparoundFinding(label string, age int64) engine.Finding {
	f := engine.Finding{Check: c.Name(), Target: label + " [wraparound]"}
	switch {
	case c.cfg.WraparoundCritAge > 0 && age >= c.cfg.WraparoundCritAge:
		f.Status, f.Message = engine.BAD, fmt.Sprintf("age(datfrozenxid) %d oltre soglia critica (%d): rischio wraparound", age, c.cfg.WraparoundCritAge)
	case c.cfg.WraparoundWarnAge > 0 && age >= c.cfg.WraparoundWarnAge:
		f.Status, f.Message = engine.WARN, fmt.Sprintf("age(datfrozenxid) %d oltre soglia (%d): vacuum in ritardo?", age, c.cfg.WraparoundWarnAge)
	default:
		f.Status, f.Message = engine.OK, fmt.Sprintf("age(datfrozenxid) %d", age)
	}
	return f
}

func (c *Check) connectionsFinding(label string, m metrics) engine.Finding {
	f := engine.Finding{Check: c.Name(), Target: label + " [connections]"}
	if m.MaxConnections <= 0 {
		f.Status, f.Message = engine.OK, fmt.Sprintf("%d connessioni", m.Connections)
		return f
	}
	pct := m.Connections * 100 / m.MaxConnections
	if c.cfg.ConnWarnPct > 0 && pct >= c.cfg.ConnWarnPct {
		f.Status = engine.WARN
	} else {
		f.Status = engine.OK
	}
	f.Message = fmt.Sprintf("%d/%d connessioni (%d%%)", m.Connections, m.MaxConnections, pct)
	return f
}

func (c *Check) slotFindings(label string, slots []slot) []engine.Finding {
	var findings []engine.Finding
	for _, s := range slots {
		f := engine.Finding{Check: c.Name(), Target: label + " [slot:" + s.Name + "]"}
		switch {
		case c.cfg.SlotCritBytes > 0 && s.RetainedBytes >= c.cfg.SlotCritBytes:
			f.Status, f.Message = engine.BAD, fmt.Sprintf("slot inattivo trattiene %s di WAL (soglia crit %s)", humanBytes(s.RetainedBytes), humanBytes(c.cfg.SlotCritBytes))
		case c.cfg.SlotWarnBytes > 0 && s.RetainedBytes >= c.cfg.SlotWarnBytes:
			f.Status, f.Message = engine.WARN, fmt.Sprintf("slot inattivo trattiene %s di WAL (soglia %s)", humanBytes(s.RetainedBytes), humanBytes(c.cfg.SlotWarnBytes))
		default:
			f.Status, f.Message = engine.WARN, fmt.Sprintf("slot inattivo (%s di WAL trattenuto)", humanBytes(s.RetainedBytes))
		}
		findings = append(findings, f)
	}
	return findings
}

func (c *Check) replicaFindings(label string, replicas []replica) []engine.Finding {
	var findings []engine.Finding
	for _, r := range replicas {
		f := engine.Finding{Check: c.Name(), Target: label + " [repl:" + r.Client + "]"}
		switch {
		case c.cfg.LagCritBytes > 0 && r.LagBytes >= c.cfg.LagCritBytes:
			f.Status, f.Message = engine.BAD, fmt.Sprintf("lag %s oltre soglia critica (%s), stato %s", humanBytes(r.LagBytes), humanBytes(c.cfg.LagCritBytes), r.State)
		case c.cfg.LagWarnBytes > 0 && r.LagBytes >= c.cfg.LagWarnBytes:
			f.Status, f.Message = engine.WARN, fmt.Sprintf("lag %s oltre soglia (%s), stato %s", humanBytes(r.LagBytes), humanBytes(c.cfg.LagWarnBytes), r.State)
		default:
			f.Status, f.Message = engine.OK, fmt.Sprintf("lag %s, stato %s", humanBytes(r.LagBytes), r.State)
		}
		findings = append(findings, f)
	}
	return findings
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
