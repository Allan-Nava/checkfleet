// Package vault implements a health check for HashiCorp Vault over its HTTP API:
// seal status (sealed / uninitialized / unseal progress) and node role
// (active / standby) with the Vault version. Read-only, HTTP/JSON, zero
// third-party dependency.
package vault

import (
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
	cfg engine.VaultConfig
}

func New(cfg engine.VaultConfig) *Check { return &Check{cfg: cfg} }

func (c *Check) Name() string { return "vault" }

type sealStatus struct {
	Sealed      bool   `json:"sealed"`
	Initialized bool   `json:"initialized"`
	T           int    `json:"t"`        // unseal threshold
	N           int    `json:"n"`        // total key shares
	Progress    int    `json:"progress"` // unseal keys provided so far
	Version     string `json:"version"`
}

type healthStatus struct {
	Sealed  bool   `json:"sealed"`
	Standby bool   `json:"standby"`
	Version string `json:"version"`
}

func (c *Check) Run(ctx context.Context) []engine.Finding {
	var findings []engine.Finding
	for _, t := range c.cfg.Targets {
		findings = append(findings, c.probe(ctx, t))
	}
	return findings
}

func (c *Check) probe(ctx context.Context, t engine.VaultTarget) engine.Finding {
	label := t.Name
	if label == "" {
		label = hostOf(t.URL)
	}
	client := c.clientFor(t)
	f := engine.Finding{Check: c.Name(), Target: label}

	// seal-status is unauthenticated and always answers 200 when reachable.
	var ss sealStatus
	if err := c.get(ctx, client, t, "/v1/sys/seal-status", &ss); err != nil {
		f.Status, f.Message = engine.ERROR, fmt.Sprintf("seal-status unreachable: %v", err)
		return f
	}
	if !ss.Initialized {
		f.Status, f.Message = engine.BAD, "Vault is not initialized"
		return f
	}
	if ss.Sealed {
		f.Status, f.Message = engine.BAD, fmt.Sprintf("Vault is sealed (unseal progress %d/%d)", ss.Progress, ss.T)
		return f
	}

	// health carries the active/standby role. Vault returns a non-200 status for
	// standby/uninitialized nodes but still a JSON body, so we read it regardless.
	var hs healthStatus
	_ = c.get(ctx, client, t, "/v1/sys/health?standbyok=true&perfstandbyok=true", &hs)

	role := "active"
	if hs.Standby {
		role = "standby"
	}
	version := ss.Version
	if version == "" {
		version = hs.Version
	}
	f.Status, f.Message = engine.OK, fmt.Sprintf("unsealed, %s, Vault %s", role, version)
	return f
}

func (c *Check) get(ctx context.Context, client *http.Client, t engine.VaultTarget, path string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(t.URL, "/")+path, nil)
	if err != nil {
		return err
	}
	if tok := os.Getenv(t.TokenEnv); t.TokenEnv != "" && tok != "" {
		req.Header.Set("X-Vault-Token", tok)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	// Vault encodes its status in the body even on 429/473/503, so parse the JSON
	// rather than treating a non-200 as an error.
	return json.Unmarshal(body, dst)
}

func (c *Check) clientFor(t engine.VaultTarget) *http.Client {
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
