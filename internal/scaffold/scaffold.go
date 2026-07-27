// Package scaffold generates a starter checkfleet.yml for `checkfleet init`.
// It emits a commented, ready-to-edit config for a chosen set of modules,
// reusing example values that parse and validate.
package scaffold

import (
	"fmt"
	"sort"
	"strings"
)

// snippet is the block emitted under `checks:` for one module (already indented
// two spaces). Every snippet carries a placeholder target so the generated file
// loads and validates as-is; the user edits the values.
var snippets = map[string]string{
	"certs": `  certs:
    warn_days: 30
    crit_days: 7
    targets: [example.com:443]`,
	"http": `  http:
    targets:
      - {url: https://example.com/health, expect_status: 200, max_latency_ms: 2000}`,
	"tcp": `  tcp:
    targets:
      - {name: db, address: db.example.com:5432, max_latency_ms: 500}`,
	"dns": `  dns:
    min_ttl_seconds: 60
    targets:
      - {name: example.com, type: A}`,
	"tls": `  tls:
    warn_days: 30
    crit_days: 7
    targets: [example.com:443]`,
	"ntp": `  ntp:
    offset_warn_ms: 100
    offset_crit_ms: 1000
    targets: [pool.ntp.org]`,
	"redis": `  redis:
    mem_warn_pct: 80
    targets: [redis.example.com]`,
	"postgres": `  postgres:
    targets:
      - {name: db, dsn: "host=db.example.com user=monitor dbname=postgres sslmode=require"}`,
	"smtp": `  smtp:
    warn_days: 30
    crit_days: 7
    targets:
      - {name: relay, address: mail.example.com:25, starttls: true}`,
	"elasticsearch": `  elasticsearch:
    disk_warn_pct: 85
    disk_crit_pct: 90
    targets:
      - {name: logs, url: https://es.example.com:9200, username: elastic, password_env: ES_PASSWORD, expect_nodes: 3}`,
	"mongodb": `  mongodb:
    lag_warn_seconds: 10
    lag_crit_seconds: 60
    targets:
      - {name: rs0, uri: "mongodb://m1.example.com:27017,m2.example.com:27017/?replicaSet=rs0", username: monitor, password_env: MONGO_PASSWORD}`,
	"nats": `  nats:
    lag_warn: 100
    lag_crit: 1000
    targets: [nats.example.com]`,
	"haproxy": `  haproxy:
    targets: [lb.example.com]`,
	"consul": `  consul:
    targets: [consul.example.com]`,
	"patroni": `  patroni:
    targets: [pg.example.com]`,
	"stream": `  stream:
    targets:
      - {url: https://cdn.example.com/live/master.m3u8, live: true}`,
	"keycloak": `  keycloak:
    base_url: https://auth.example.com
    realms: [master]`,
	"rabbitmq": `  rabbitmq:
    username: monitoring
    password_env: RABBITMQ_PASSWORD
    targets: [rabbit.example.com:15672]`,
	"grpc": `  grpc:
    targets:
      - {name: api, address: api.example.com:443, service: ""}`,
	"ldap": `  ldap:
    targets:
      - {name: dir, address: ldap.example.com:636, tls: true}`,
	"kafka": `  kafka:
    targets: [kafka.example.com:9092]`,
	"ingest": `  ingest:
    targets:
      - {name: rtmp, address: ingest.example.com:1935, protocol: rtmp, max_latency_ms: 2000}`,
	"s3": `  s3:
    targets:
      - {name: backups, endpoint: https://s3.example.com, bucket: backups, region: us-east-1, access_key_env: S3_ACCESS_KEY, secret_key_env: S3_SECRET_KEY}`,
	"mysql": `  mysql:
    conn_warn_pct: 80
    targets:
      - {name: primary, dsn: "monitor:${MYSQL_PASSWORD}@tcp(db.example.com:3306)/"}`,
	"etcd": `  etcd:
    expect_members: 3
    targets:
      - {name: etcd-01, url: https://etcd-01:2379, insecure_skip_verify: true}`,
	"clickhouse": `  clickhouse:
    delay_warn_seconds: 30
    delay_crit_seconds: 300
    targets:
      - {name: ch-01, url: http://ch-01:8123, username: monitor, password_env: CLICKHOUSE_PASSWORD}`,
	"vault": `  vault:
    targets:
      - {name: vault-01, url: https://vault-01:8200, insecure_skip_verify: true}`,
	"memcached": `  memcached:
    mem_warn_pct: 90
    evictions_warn: 0
    targets: [cache-01, cache-02:11212]`,
	"cassandra": `  cassandra:
    expect_nodes: 0
    targets:
      - {name: cass-01, address: cass-01:9042, max_latency_ms: 500}`,
}

// defaultModules is the starter set used when the user names none.
var defaultModules = []string{"certs", "http"}

// Recipe is a named starter stack: the modules that answer the questions a
// given kind of infrastructure actually raises.
//
// The point is not to save typing — it is that someone starting out does not yet
// know that a web tier needs `dns` (records drift after a migration and nothing
// else notices) or that an edge tier needs `ntp` (clock skew breaks TLS and
// tokens long before anyone suspects the clock).
type Recipe struct {
	Name    string
	Summary string
	Modules []string
}

var recipes = map[string]Recipe{
	"web": {
		Name:    "web",
		Summary: "public web tier: is it answering, is the certificate alive, do the records still point at us",
		Modules: []string{"http", "certs", "dns"},
	},
	"db": {
		Name:    "db",
		Summary: "data tier: connectivity, replication lag and memory pressure",
		Modules: []string{"postgres", "redis", "tcp"},
	},
	"edge": {
		Name:    "edge",
		Summary: "load balancers and network edge: backend health, TLS expiry, reachability and clock skew",
		Modules: []string{"haproxy", "certs", "tcp", "ntp"},
	},
	"media": {
		Name:    "media",
		Summary: "streaming: manifest freshness, live-edge age and the ingest endpoints behind it",
		Modules: []string{"stream", "ingest", "http"},
	},
}

// Recipes returns the recipe names, sorted.
func Recipes() []string {
	names := make([]string, 0, len(recipes))
	for n := range recipes {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// RecipeByName looks up a recipe.
func RecipeByName(name string) (Recipe, bool) {
	r, ok := recipes[strings.ToLower(strings.TrimSpace(name))]
	return r, ok
}

// ConfigForRecipe renders a starter config for a named stack.
func ConfigForRecipe(name string) (string, error) {
	r, ok := RecipeByName(name)
	if !ok {
		return "", fmt.Errorf("unknown recipe %q; available: %s", name, strings.Join(Recipes(), ", "))
	}
	body, err := Config(r.Modules)
	if err != nil {
		// A recipe naming a module with no snippet is a programming error, not
		// user input — RecipesAreValid covers it in the tests.
		return "", fmt.Errorf("recipe %q: %w", name, err)
	}
	header := fmt.Sprintf("# Recipe: %s — %s\n", r.Name, r.Summary)
	return header + body, nil
}

// Supported returns the module names init can scaffold, sorted.
func Supported() []string {
	names := make([]string, 0, len(snippets))
	for n := range snippets {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Config renders a starter checkfleet.yml for the given modules (empty → the
// default starter set). Unknown module names are an error. Duplicates are
// de-duplicated while preserving the caller's order.
func Config(modules []string) (string, error) {
	if len(modules) == 0 {
		modules = defaultModules
	}
	seen := map[string]bool{}
	var ordered []string
	for _, m := range modules {
		m = strings.TrimSpace(m)
		if m == "" || seen[m] {
			continue
		}
		if _, ok := snippets[m]; !ok {
			return "", fmt.Errorf("unknown module %q; supported: %s", m, strings.Join(Supported(), ", "))
		}
		seen[m] = true
		ordered = append(ordered, m)
	}
	if len(ordered) == 0 {
		return "", fmt.Errorf("no module selected")
	}

	var b strings.Builder
	b.WriteString("# checkfleet.yml — generated by `checkfleet init`.\n")
	b.WriteString("# Edit the placeholder targets/thresholds below, then run:\n")
	b.WriteString("#   checkfleet check all --config checkfleet.yml\n")
	b.WriteString("# Secrets come from the environment (never inline). See checkfleet.example.yml\n")
	b.WriteString("# for every module and option.\n\n")
	b.WriteString("timeout_seconds: 30\n\n")
	b.WriteString("checks:\n")
	for i, m := range ordered {
		b.WriteString(snippets[m])
		b.WriteString("\n")
		if i < len(ordered)-1 {
			b.WriteString("\n")
		}
	}
	return b.String(), nil
}
