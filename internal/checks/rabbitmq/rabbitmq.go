// Package rabbitmq implements a health check for RabbitMQ via its management
// HTTP API: nodes running (no memory/disk alarms) and queue depth / consumer
// presence. Read-only; HTTP/JSON, zero third-party dependency.
package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

type Check struct {
	cfg    engine.RabbitMQConfig
	client *http.Client
}

func New(cfg engine.RabbitMQConfig) *Check {
	return &Check{cfg: cfg, client: &http.Client{}}
}

func (c *Check) Name() string { return "rabbitmq" }

type overview struct {
	RabbitMQVersion string `json:"rabbitmq_version"`
	ClusterName     string `json:"cluster_name"`
}

type node struct {
	Name          string `json:"name"`
	Running       bool   `json:"running"`
	MemAlarm      bool   `json:"mem_alarm"`
	DiskFreeAlarm bool   `json:"disk_free_alarm"`
}

type queue struct {
	Name      string `json:"name"`
	Vhost     string `json:"vhost"`
	Messages  int    `json:"messages"`
	Consumers int    `json:"consumers"`
}

func (c *Check) Run(ctx context.Context) []engine.Finding {
	var findings []engine.Finding
	// Any management node answers cluster-wide; use the first reachable one and
	// report ERROR for the unreachable endpoints.
	base := ""
	for _, t := range c.cfg.Targets {
		target := withDefaultPort(t, c.cfg.Port)
		var ov overview
		if err := c.get(ctx, target, "/api/overview", &ov); err != nil {
			findings = append(findings, engine.Finding{Check: c.Name(), Target: target, Status: engine.ERROR, Message: fmt.Sprintf("management API not reachable: %v", err)})
			continue
		}
		if base == "" {
			base = target
			findings = append(findings, engine.Finding{Check: c.Name(), Target: target, Status: engine.OK, Message: "RabbitMQ v" + ov.RabbitMQVersion})
		}
	}
	if base == "" {
		return findings
	}
	findings = append(findings, c.nodeFindings(ctx, base)...)
	findings = append(findings, c.queueFindings(ctx, base)...)
	return findings
}

func (c *Check) nodeFindings(ctx context.Context, base string) []engine.Finding {
	var nodes []node
	if err := c.get(ctx, base, "/api/nodes", &nodes); err != nil {
		return []engine.Finding{{Check: c.Name(), Target: "nodes", Status: engine.ERROR, Message: err.Error()}}
	}
	var findings []engine.Finding
	for _, n := range nodes {
		f := engine.Finding{Check: c.Name(), Target: "node/" + shortNode(n.Name)}
		switch {
		case !n.Running:
			f.Status, f.Message = engine.BAD, "node not running"
		case n.MemAlarm:
			f.Status, f.Message = engine.BAD, "memory alarm active"
		case n.DiskFreeAlarm:
			f.Status, f.Message = engine.BAD, "disk free alarm active"
		default:
			f.Status, f.Message = engine.OK, "running"
		}
		findings = append(findings, f)
	}
	return findings
}

func (c *Check) queueFindings(ctx context.Context, base string) []engine.Finding {
	var queues []queue
	if err := c.get(ctx, base, "/api/queues", &queues); err != nil {
		return []engine.Finding{{Check: c.Name(), Target: "queues", Status: engine.ERROR, Message: err.Error()}}
	}
	var findings []engine.Finding
	for _, q := range queues {
		label := "queue/" + strings.TrimPrefix(q.Vhost, "/") + "/" + q.Name
		if strings.HasPrefix(label, "queue//") {
			label = "queue/" + q.Name
		}
		f := engine.Finding{Check: c.Name(), Target: label}
		switch {
		case c.cfg.QueueCritDepth > 0 && q.Messages >= c.cfg.QueueCritDepth:
			f.Status, f.Message = engine.BAD, fmt.Sprintf("%d messages queued (over %d)", q.Messages, c.cfg.QueueCritDepth)
		case c.cfg.QueueWarnDepth > 0 && q.Messages >= c.cfg.QueueWarnDepth:
			f.Status, f.Message = engine.WARN, fmt.Sprintf("%d messages queued (over %d)", q.Messages, c.cfg.QueueWarnDepth)
		case q.Messages > 0 && q.Consumers == 0:
			f.Status, f.Message = engine.WARN, fmt.Sprintf("%d messages but no consumer", q.Messages)
		default:
			f.Status, f.Message = engine.OK, fmt.Sprintf("%d messages, %d consumers", q.Messages, q.Consumers)
		}
		findings = append(findings, f)
	}
	return findings
}

func (c *Check) get(ctx context.Context, target, path string, dst any) error {
	url := c.scheme() + "://" + target + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.cfg.Username, os.Getenv(c.cfg.PasswordEnv))
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return err
	}
	return json.Unmarshal(body, dst)
}

func (c *Check) scheme() string {
	if c.cfg.Scheme != "" {
		return c.cfg.Scheme
	}
	return "http"
}

// shortNode trims the "rabbit@" prefix from a node name.
func shortNode(name string) string {
	if i := strings.IndexByte(name, '@'); i >= 0 {
		return name[i+1:]
	}
	return name
}

func withDefaultPort(target string, port int) string {
	if strings.Contains(target, ":") {
		return target
	}
	return fmt.Sprintf("%s:%d", target, port)
}
