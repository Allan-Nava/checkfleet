// Package kafka implements a health check for a Kafka cluster: a controller is
// present, the broker count meets expectations, no under-replicated
// partitions, and configured consumer groups aren't lagging. The Kafka I/O is
// behind the cluster interface so the finding logic is unit-tested with a
// fake; the franz-go/kadm-backed cluster lives in kadm.go.
package kafka

import (
	"context"
	"fmt"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// metadata is the cluster snapshot the check evaluates.
type metadata struct {
	Brokers           int
	ControllerPresent bool
	UnderReplicated   int
}

// cluster is the Kafka I/O this check needs.
type cluster interface {
	Metadata(ctx context.Context) (metadata, error)
	GroupLag(ctx context.Context, group string) (int64, error)
	Close()
}

type Check struct {
	cfg engine.KafkaConfig
	// connect is injectable for tests; defaults to the franz-go/kadm cluster.
	connect func(ctx context.Context, cfg engine.KafkaConfig) (cluster, error)
}

func New(cfg engine.KafkaConfig) *Check {
	return &Check{cfg: cfg, connect: connectKadm}
}

func (c *Check) Name() string { return "kafka" }

func (c *Check) Run(ctx context.Context) []engine.Finding {
	cl, err := c.connect(ctx, c.cfg)
	if err != nil {
		return []engine.Finding{{Check: c.Name(), Target: "cluster", Status: engine.ERROR, Message: fmt.Sprintf("connection failed: %v", err)}}
	}
	defer cl.Close()

	var findings []engine.Finding
	meta, err := cl.Metadata(ctx)
	if err != nil {
		findings = append(findings, engine.Finding{Check: c.Name(), Target: "cluster", Status: engine.ERROR, Message: fmt.Sprintf("metadata failed: %v", err)})
	} else {
		findings = append(findings, c.clusterFinding(meta), c.partitionsFinding(meta))
	}
	for _, g := range c.cfg.Groups {
		findings = append(findings, c.groupFinding(ctx, cl, g))
	}
	return findings
}

func (c *Check) clusterFinding(m metadata) engine.Finding {
	f := engine.Finding{Check: c.Name(), Target: "cluster"}
	switch {
	case !m.ControllerPresent:
		f.Status, f.Message = engine.BAD, fmt.Sprintf("no controller (%d brokers)", m.Brokers)
	case c.cfg.ExpectBrokers > 0 && m.Brokers < c.cfg.ExpectBrokers:
		f.Status, f.Message = engine.WARN, fmt.Sprintf("%d/%d brokers (missing)", m.Brokers, c.cfg.ExpectBrokers)
	default:
		f.Status, f.Message = engine.OK, fmt.Sprintf("%d brokers, controller present", m.Brokers)
	}
	return f
}

func (c *Check) partitionsFinding(m metadata) engine.Finding {
	f := engine.Finding{Check: c.Name(), Target: "partitions"}
	if m.UnderReplicated > 0 {
		f.Status, f.Message = engine.BAD, fmt.Sprintf("%d under-replicated partitions", m.UnderReplicated)
	} else {
		f.Status, f.Message = engine.OK, "no under-replicated partitions"
	}
	return f
}

func (c *Check) groupFinding(ctx context.Context, cl cluster, group string) engine.Finding {
	f := engine.Finding{Check: c.Name(), Target: "group/" + group}
	lag, err := cl.GroupLag(ctx, group)
	if err != nil {
		f.Status, f.Message = engine.ERROR, fmt.Sprintf("lag unavailable: %v", err)
		return f
	}
	switch {
	case c.cfg.LagCrit > 0 && lag >= c.cfg.LagCrit:
		f.Status, f.Message = engine.BAD, fmt.Sprintf("total lag %d (over %d)", lag, c.cfg.LagCrit)
	case c.cfg.LagWarn > 0 && lag >= c.cfg.LagWarn:
		f.Status, f.Message = engine.WARN, fmt.Sprintf("total lag %d (over %d)", lag, c.cfg.LagWarn)
	default:
		f.Status, f.Message = engine.OK, fmt.Sprintf("total lag %d", lag)
	}
	return f
}
