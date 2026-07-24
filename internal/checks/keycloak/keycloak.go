// Package keycloak implements a health check for Keycloak: the health endpoint
// reports UP, and each configured realm serves a valid OIDC discovery document
// with a coherent issuer and a token endpoint. HTTP/JSON only, read-only.
package keycloak

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

type Check struct {
	cfg    engine.KeycloakConfig
	client *http.Client
}

func New(cfg engine.KeycloakConfig) *Check {
	return &Check{cfg: cfg, client: &http.Client{}}
}

func (c *Check) Name() string { return "keycloak" }

type healthResp struct {
	Status string `json:"status"`
}

type oidcConfig struct {
	Issuer        string `json:"issuer"`
	TokenEndpoint string `json:"token_endpoint"`
}

func (c *Check) Run(ctx context.Context) []engine.Finding {
	var findings []engine.Finding
	if c.cfg.HealthURL != "" {
		findings = append(findings, c.healthFinding(ctx))
	}
	base := strings.TrimRight(c.cfg.BaseURL, "/")
	for _, realm := range c.cfg.Realms {
		findings = append(findings, c.realmFinding(ctx, base, realm))
	}
	return findings
}

func (c *Check) healthFinding(ctx context.Context) engine.Finding {
	f := engine.Finding{Check: c.Name(), Target: "health"}
	body, _, err := c.get(ctx, c.cfg.HealthURL)
	if err != nil {
		f.Status, f.Message = engine.ERROR, fmt.Sprintf("health not reachable: %v", err)
		return f
	}
	var h healthResp
	if err := json.Unmarshal(body, &h); err != nil {
		f.Status, f.Message = engine.BAD, "invalid health response"
		return f
	}
	if !strings.EqualFold(h.Status, "UP") {
		f.Status, f.Message = engine.BAD, "health status: "+h.Status
		return f
	}
	f.Status, f.Message = engine.OK, "health UP"
	return f
}

func (c *Check) realmFinding(ctx context.Context, base, realm string) engine.Finding {
	f := engine.Finding{Check: c.Name(), Target: "realm/" + realm}
	url := base + "/realms/" + realm + "/.well-known/openid-configuration"
	body, code, err := c.get(ctx, url)
	if err != nil {
		f.Status, f.Message = engine.ERROR, fmt.Sprintf("discovery not reachable: %v", err)
		return f
	}
	if code != http.StatusOK {
		f.Status, f.Message = engine.BAD, fmt.Sprintf("discovery HTTP %d (realm missing?)", code)
		return f
	}
	var oidc oidcConfig
	if err := json.Unmarshal(body, &oidc); err != nil || oidc.TokenEndpoint == "" {
		f.Status, f.Message = engine.BAD, "invalid OIDC discovery or missing token_endpoint"
		return f
	}
	// The issuer should end with /realms/<realm>; a mismatch usually means a
	// proxy/frontend-URL misconfiguration.
	if !strings.HasSuffix(strings.TrimRight(oidc.Issuer, "/"), "/realms/"+realm) {
		f.Status, f.Message = engine.WARN, fmt.Sprintf("unexpected issuer: %s (want .../realms/%s)", oidc.Issuer, realm)
		return f
	}
	f.Status, f.Message = engine.OK, "OIDC ok, token_endpoint present"
	return f
}

// get fetches url, returning body and status. Non-2xx (other than 404, left to
// the caller) is an error.
func (c *Check) get(ctx context.Context, url string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 500 {
		return body, resp.StatusCode, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return body, resp.StatusCode, nil
}
