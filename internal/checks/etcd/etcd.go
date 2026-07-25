// Package etcd implements a health check for an etcd v3 cluster over its HTTP
// JSON gateway (no clientv3 dependency): /health, maintenance status (leader
// present, version) and member count. Zero third-party dependency.
package etcd

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

type Check struct {
	cfg engine.EtcdConfig
}

func New(cfg engine.EtcdConfig) *Check { return &Check{cfg: cfg} }

func (c *Check) Name() string { return "etcd" }

// etcd's JSON gateway encodes 64-bit ints (leader, member ids) as strings.
type healthResp struct {
	Health string `json:"health"`
}
type statusResp struct {
	Version string `json:"version"`
	Leader  string `json:"leader"`
}
type memberListResp struct {
	Members []struct {
		ID   string `json:"ID"`
		Name string `json:"name"`
	} `json:"members"`
}
type authResp struct {
	Token string `json:"token"`
}

func (c *Check) Run(ctx context.Context) []engine.Finding {
	var findings []engine.Finding
	for _, t := range c.cfg.Targets {
		findings = append(findings, c.probe(ctx, t)...)
	}
	return findings
}

func (c *Check) probe(ctx context.Context, t engine.EtcdTarget) []engine.Finding {
	label := t.Name
	if label == "" {
		label = hostOf(t.URL)
	}
	client := c.clientFor(t)

	token, err := c.authenticate(ctx, client, t)
	if err != nil {
		return []engine.Finding{{Check: c.Name(), Target: label, Status: engine.ERROR,
			Message: fmt.Sprintf("authentication failed: %v", err)}}
	}

	var h healthResp
	if err := c.get(ctx, client, t, token, "/health", &h); err != nil {
		return []engine.Finding{{Check: c.Name(), Target: label, Status: engine.ERROR,
			Message: fmt.Sprintf("health unreachable: %v", err)}}
	}
	if !strings.EqualFold(h.Health, "true") {
		return []engine.Finding{{Check: c.Name(), Target: label, Status: engine.BAD,
			Message: "endpoint reports unhealthy"}}
	}

	var st statusResp
	if err := c.post(ctx, client, t, token, "/v3/maintenance/status", &st); err != nil {
		return []engine.Finding{{Check: c.Name(), Target: label, Status: engine.ERROR,
			Message: fmt.Sprintf("status query failed: %v", err)}}
	}
	if st.Leader == "" || st.Leader == "0" {
		return []engine.Finding{{Check: c.Name(), Target: label, Status: engine.BAD,
			Message: "no leader (cluster has lost quorum)"}}
	}

	members := 0
	var ml memberListResp
	if err := c.post(ctx, client, t, token, "/v3/cluster/member/list", &ml); err == nil {
		members = len(ml.Members)
	}

	findings := []engine.Finding{{Check: c.Name(), Target: label, Status: engine.OK,
		Message: fmt.Sprintf("healthy, etcd %s, leader present, %d members", st.Version, members)}}

	if c.cfg.ExpectMembers > 0 && members > 0 && members < c.cfg.ExpectMembers {
		findings = append(findings, engine.Finding{Check: c.Name(), Target: label + "/members",
			Status: engine.BAD,
			Message: fmt.Sprintf("%d members present, expected %d (quorum risk)", members, c.cfg.ExpectMembers)})
	}
	return findings
}

// authenticate exchanges username/password for a token when credentials are set;
// returns "" (no auth header) otherwise.
func (c *Check) authenticate(ctx context.Context, client *http.Client, t engine.EtcdTarget) (string, error) {
	if t.Username == "" {
		return "", nil
	}
	body, _ := json.Marshal(map[string]string{"name": t.Username, "password": os.Getenv(t.PasswordEnv)})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(t.URL, "/")+"/v3/auth/authenticate", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var ar authResp
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
		return "", err
	}
	return ar.Token, nil
}

func (c *Check) get(ctx context.Context, client *http.Client, t engine.EtcdTarget, token, path string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(t.URL, "/")+path, nil)
	if err != nil {
		return err
	}
	return c.do(client, req, token, dst)
}

func (c *Check) post(ctx context.Context, client *http.Client, t engine.EtcdTarget, token, path string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(t.URL, "/")+path, strings.NewReader("{}"))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(client, req, token, dst)
}

func (c *Check) do(client *http.Client, req *http.Request, token string, dst any) error {
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	return json.Unmarshal(body, dst)
}

func (c *Check) clientFor(t engine.EtcdTarget) *http.Client {
	if t.Insecure {
		return &http.Client{Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}}
	}
	return &http.Client{}
}

func hostOf(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return u.Host
	}
	return raw
}
