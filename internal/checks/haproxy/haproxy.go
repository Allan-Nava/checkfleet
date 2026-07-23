// Package haproxy implements a backend/server health check for HAProxy,
// reading the CSV stats export over HTTP (the `;csv` stats endpoint). It flags
// servers that are DOWN (BAD) or in MAINT/DRAIN/NOLB (WARN), backends with no
// available server (BAD), and — optionally — session usage near the limit.
//
// Only the read-only stats page is consumed; the check never mutates HAProxy.
package haproxy

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/Allan-Nava/checkfleet/internal/inventory"
)

type Check struct {
	cfg    engine.HAProxyConfig
	client *http.Client
}

func New(cfg engine.HAProxyConfig) *Check {
	return &Check{cfg: cfg, client: &http.Client{}}
}

func (c *Check) Name() string { return "haproxy" }

// Targets resolves explicit targets plus inventory hosts to host:port pairs.
func (c *Check) Targets() ([]string, error) {
	var targets []string
	for _, t := range c.cfg.Targets {
		targets = append(targets, withDefaultPort(t, c.cfg.Port))
	}
	if c.cfg.AnsibleInventory != "" {
		hosts, err := inventory.LoadPath(c.cfg.AnsibleInventory)
		if err != nil {
			return targets, fmt.Errorf("inventory %s: %w", c.cfg.AnsibleInventory, err)
		}
		for _, h := range hosts {
			targets = append(targets, withDefaultPort(h.Address, c.cfg.Port))
		}
	}
	return targets, nil
}

func (c *Check) Run(ctx context.Context) []engine.Finding {
	targets, err := c.Targets()
	var findings []engine.Finding
	if err != nil {
		findings = append(findings, engine.Finding{
			Check: c.Name(), Target: c.cfg.AnsibleInventory, Status: engine.ERROR, Message: err.Error(),
		})
	}
	// One target's stats page can list many backends/servers, so the number of
	// findings is unknown up front: collect per target, then flatten.
	perTarget := make([][]engine.Finding, len(targets))
	sem := make(chan struct{}, 16)
	done := make(chan int)
	for i, target := range targets {
		go func(i int, target string) {
			sem <- struct{}{}
			perTarget[i] = c.probe(ctx, target)
			<-sem
			done <- i
		}(i, target)
	}
	for range targets {
		<-done
	}
	for _, fs := range perTarget {
		findings = append(findings, fs...)
	}
	return findings
}

func (c *Check) probe(ctx context.Context, target string) []engine.Finding {
	rows, err := c.fetchStats(ctx, target)
	if err != nil {
		return []engine.Finding{{
			Check: c.Name(), Target: target, Status: engine.ERROR,
			Message: fmt.Sprintf("stats non raggiungibili: %v", err),
		}}
	}
	var findings []engine.Finding
	for _, row := range rows {
		pxname, svname := row["pxname"], row["svname"]
		if svname == "" || svname == "FRONTEND" {
			continue // frontends aren't backend health; skip to stay signal-dense
		}
		findings = append(findings, c.rowFinding(target, pxname, svname, row))
	}
	if len(findings) == 0 {
		findings = append(findings, engine.Finding{
			Check: c.Name(), Target: target, Status: engine.WARN,
			Message: "nessun backend/server nelle stats",
		})
	}
	return findings
}

func (c *Check) rowFinding(target, pxname, svname string, row map[string]string) engine.Finding {
	label := fmt.Sprintf("%s/%s", pxname, svname)
	status := strings.TrimSpace(row["status"])
	f := engine.Finding{Check: c.Name(), Target: label}

	switch {
	case svname == "BACKEND" && strings.HasPrefix(status, "DOWN"):
		f.Status, f.Message = engine.BAD, "backend DOWN: nessun server disponibile"
		return f
	case strings.HasPrefix(status, "DOWN"):
		f.Status, f.Message = engine.BAD, "server DOWN"
		return f
	case strings.HasPrefix(status, "MAINT"):
		f.Status, f.Message = engine.WARN, "server in MAINT (manutenzione)"
		return f
	case strings.HasPrefix(status, "DRAIN"):
		f.Status, f.Message = engine.WARN, "server in DRAIN"
		return f
	case strings.HasPrefix(status, "NOLB"):
		f.Status, f.Message = engine.WARN, "server NOLB (fuori dal load balancing)"
		return f
	}

	// Healthy (UP / OPEN / no check): optionally warn on session saturation.
	if pct, ok := c.sessionUsage(row); ok && c.cfg.SessionWarnPct > 0 && pct >= c.cfg.SessionWarnPct {
		f.Status = engine.WARN
		f.Message = fmt.Sprintf("%s, sessioni al %d%% del limite (%s/%s)", statusOrUp(status), pct, row["scur"], row["slim"])
		return f
	}
	f.Status, f.Message = engine.OK, statusOrUp(status)
	return f
}

// sessionUsage returns the percent of the session limit in use (scur/slim).
func (c *Check) sessionUsage(row map[string]string) (int, bool) {
	scur, err1 := strconv.Atoi(strings.TrimSpace(row["scur"]))
	slim, err2 := strconv.Atoi(strings.TrimSpace(row["slim"]))
	if err1 != nil || err2 != nil || slim <= 0 {
		return 0, false
	}
	return scur * 100 / slim, true
}

func (c *Check) fetchStats(ctx context.Context, target string) ([]map[string]string, error) {
	url := c.scheme() + "://" + target + c.path()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if c.cfg.AuthUser != "" {
		req.SetBasicAuth(c.cfg.AuthUser, os.Getenv(c.cfg.AuthPassEnv))
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return parseCSV(io.LimitReader(resp.Body, 16<<20))
}

// parseCSV parses the HAProxy stats CSV. Its first line is a header comment
// like "# pxname,svname,...": we strip the "# " and key each row by column name.
func parseCSV(r io.Reader) ([]map[string]string, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1 // rows have a trailing empty field
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("CSV vuoto")
	}
	header := records[0]
	if len(header) > 0 {
		header[0] = strings.TrimPrefix(strings.TrimSpace(header[0]), "# ")
	}
	var rows []map[string]string
	for _, rec := range records[1:] {
		if len(rec) == 0 || (len(rec) == 1 && rec[0] == "") {
			continue
		}
		row := make(map[string]string, len(header))
		for i, name := range header {
			if i < len(rec) {
				row[name] = rec[i]
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (c *Check) scheme() string {
	if c.cfg.Scheme != "" {
		return c.cfg.Scheme
	}
	return "http"
}

func (c *Check) path() string {
	if c.cfg.Path != "" {
		return c.cfg.Path
	}
	return "/stats;csv"
}

func statusOrUp(status string) string {
	if status == "" {
		return "UP"
	}
	return status
}

func withDefaultPort(target string, port int) string {
	if strings.Contains(target, ":") {
		return target
	}
	return fmt.Sprintf("%s:%d", target, port)
}
