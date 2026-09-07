package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// consulHTTP is the client used for catalog lookups. Short timeout on purpose:
// discovery runs before the checks, so a hanging catalog must not eat the run's
// budget before a single target has been probed.
var consulHTTP = &http.Client{Timeout: 10 * time.Second}

// healthEntry is the slice of /v1/health/service/<name> we need. The catalog
// carries far more; decoding only these fields keeps the dependency on Consul's
// response shape as small as it can be.
type healthEntry struct {
	Node struct {
		Node    string `json:"Node"`
		Address string `json:"Address"`
	} `json:"Node"`
	Service struct {
		Address string   `json:"Address"`
		Port    int      `json:"Port"`
		Tags    []string `json:"Tags"`
	} `json:"Service"`
}

// consulCatalog lists the instances of a service.
//
// It talks to the catalog directly rather than reusing internal/checks/consul:
// that module's client is a method on its own Check, wired to that module's
// config and TLS, and exporting it would couple target discovery to a check's
// lifecycle. This is a dozen lines of the same zero-dep HTTP+JSON.
func consulCatalog(ctx context.Context, cs engine.ConsulService) ([]Host, error) {
	if cs.Service == "" {
		return nil, fmt.Errorf("service is required")
	}
	addr := cs.Address
	if addr == "" {
		addr = "127.0.0.1:8500"
	} else if !strings.Contains(addr, ":") {
		addr += ":8500"
	}
	scheme := cs.Scheme
	if scheme == "" {
		scheme = "http"
	}

	q := url.Values{}
	if cs.Healthy() {
		// Only instances passing their own checks. A target list carrying
		// known-dead nodes turns one outage into a second, noisier one.
		q.Set("passing", "true")
	}
	if cs.Tag != "" {
		q.Set("tag", cs.Tag)
	}
	endpoint := scheme + "://" + addr + "/v1/health/service/" + url.PathEscape(cs.Service)
	if len(q) > 0 {
		endpoint += "?" + q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if cs.TokenEnv != "" {
		if tok := os.Getenv(cs.TokenEnv); tok != "" {
			req.Header.Set("X-Consul-Token", tok)
		}
	}
	resp, err := consulHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var entries []healthEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("decoding catalog: %w", err)
	}

	hosts := make([]Host, 0, len(entries))
	for _, e := range entries {
		// A service may advertise its own address (a container's routable IP);
		// fall back to the node's when it does not, which is the common case.
		host := e.Service.Address
		if host == "" {
			host = e.Node.Address
		}
		if host == "" {
			continue
		}
		addr := host
		if e.Service.Port > 0 && cs.Port() {
			addr = net.JoinHostPort(host, strconv.Itoa(e.Service.Port))
		}
		name := e.Node.Node
		if name == "" {
			name = host
		}
		hosts = append(hosts, Host{Name: name, Address: addr, Source: "consul"})
	}
	return hosts, nil
}
