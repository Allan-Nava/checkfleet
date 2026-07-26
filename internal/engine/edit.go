package engine

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// EndpointSpec is a single endpoint added via the desktop "Add endpoint"
// quick-form. Only the fields relevant to Kind are read.
type EndpointSpec struct {
	Kind         string `json:"kind"`         // http | certs | tls | tcp | dns | redis | nats | smtp | grpc | postgres
	Value        string `json:"value"`        // url (http) | host:port (certs/tls/tcp/redis/nats/smtp/grpc) | name (dns) | dsn (postgres)
	ExpectStatus int    `json:"expectStatus"` // http only; 0 omits the key
	RecordType   string `json:"recordType"`   // dns only; "" or "A" omits the key
	Extra        string `json:"extra"`        // grpc service / postgres password_env; "" omits the key
}

// AddEndpoint inserts a new endpoint into raw config YAML and returns the
// updated text. It edits the YAML node tree so existing comments, key order and
// formatting are preserved; only the touched module gains the new target. An
// empty document is turned into a fresh mapping. The result is not validated
// here — the caller can Validate/LoadBytes it.
func AddEndpoint(yamlText string, spec EndpointSpec) (string, error) {
	spec.Kind = strings.TrimSpace(spec.Kind)
	spec.Value = strings.TrimSpace(spec.Value)
	if spec.Value == "" {
		return "", fmt.Errorf("endpoint value is empty")
	}
	switch spec.Kind {
	case "http", "certs", "tls", "tcp", "dns", "redis", "nats", "smtp", "grpc", "postgres":
	default:
		return "", fmt.Errorf("unsupported endpoint kind %q", spec.Kind)
	}

	var doc yaml.Node
	if strings.TrimSpace(yamlText) == "" {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	} else {
		if err := yaml.Unmarshal([]byte(yamlText), &doc); err != nil {
			return "", err
		}
		if len(doc.Content) == 0 { // whitespace/comments only
			doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
		}
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return "", fmt.Errorf("top-level YAML is not a mapping")
	}

	checks := ensureMap(root, "checks")
	module := ensureMap(checks, spec.Kind)
	targets := ensureSeq(module, "targets")

	switch spec.Kind {
	case "certs", "tls", "redis", "nats":
		// targets is a sequence of host[:port] (or monitor URL) scalars.
		targets.Content = append(targets.Content, scalarNode(spec.Value))
	case "http":
		m := &yaml.Node{Kind: yaml.MappingNode}
		m.Content = append(m.Content, keyNode("url"), scalarNode(spec.Value))
		if spec.ExpectStatus > 0 {
			m.Content = append(m.Content, keyNode("expect_status"), intNode(spec.ExpectStatus))
		}
		targets.Content = append(targets.Content, m)
	case "tcp", "smtp":
		m := &yaml.Node{Kind: yaml.MappingNode}
		m.Content = append(m.Content, keyNode("address"), scalarNode(spec.Value))
		targets.Content = append(targets.Content, m)
	case "grpc":
		m := &yaml.Node{Kind: yaml.MappingNode}
		m.Content = append(m.Content, keyNode("address"), scalarNode(spec.Value))
		if svc := strings.TrimSpace(spec.Extra); svc != "" {
			m.Content = append(m.Content, keyNode("service"), scalarNode(svc))
		}
		targets.Content = append(targets.Content, m)
	case "postgres":
		m := &yaml.Node{Kind: yaml.MappingNode}
		m.Content = append(m.Content, keyNode("dsn"), scalarNode(spec.Value))
		if pw := strings.TrimSpace(spec.Extra); pw != "" {
			m.Content = append(m.Content, keyNode("password_env"), scalarNode(pw))
		}
		targets.Content = append(targets.Content, m)
	case "dns":
		m := &yaml.Node{Kind: yaml.MappingNode}
		m.Content = append(m.Content, keyNode("name"), scalarNode(spec.Value))
		if rt := strings.ToUpper(strings.TrimSpace(spec.RecordType)); rt != "" && rt != "A" {
			m.Content = append(m.Content, keyNode("type"), scalarNode(rt))
		}
		targets.Content = append(targets.Content, m)
	}

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// mapValue returns the value node for key in a mapping node, or nil.
func mapValue(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// ensureMap returns the mapping node stored under key in m, creating an empty
// one (and the key) when absent.
func ensureMap(m *yaml.Node, key string) *yaml.Node {
	if v := mapValue(m, key); v != nil {
		return v
	}
	v := &yaml.Node{Kind: yaml.MappingNode}
	m.Content = append(m.Content, keyNode(key), v)
	return v
}

// ensureSeq returns the sequence node stored under key in m, creating an empty
// one (and the key) when absent.
func ensureSeq(m *yaml.Node, key string) *yaml.Node {
	if v := mapValue(m, key); v != nil {
		return v
	}
	v := &yaml.Node{Kind: yaml.SequenceNode}
	m.Content = append(m.Content, keyNode(key), v)
	return v
}

func keyNode(s string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s}
}

func scalarNode(s string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s}
}

func intNode(n int) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(n)}
}
