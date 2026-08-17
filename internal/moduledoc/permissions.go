package moduledoc

// Permission is the least privilege a module needs on the system it checks
// (CF-181), plus what it explicitly does *not* need.
//
// The second half is the point. A security review does not ask "what does it
// need"; it asks "what can it do", and an answer that lists only grants leaves
// the reviewer to assume the worst. Saying "no read access to any user table,
// no write of any kind" is what gets the ticket approved.
//
// These are derived from the queries and endpoints each module actually issues.
// When a module gains a call, its entry here has to move with it — the gate in
// permissions_test.go fails when a registry module has no entry at all, and the
// integration suite proves the postgres/mysql/mongodb ones by creating a user
// with exactly these grants and nothing else.
type Permission struct {
	// Summary is one line: what access the check needs to do its job.
	Summary string
	// Statements are copy-pasteable, when the system has such a thing. Empty
	// for systems configured by file or by an unauthenticated endpoint.
	Statements []string
	// NotNeeded names the access the check does not require.
	NotNeeded string
	// Unauthenticated marks a module that needs no credential at all.
	Unauthenticated bool
	// NeedsJudgement marks an entry where the exact policy depends on how the
	// target is deployed, so the text is guidance rather than a recipe. Being
	// explicit about it beats printing a confident statement that does not fit.
	NeedsJudgement bool
}

// Permissions maps a module name to its least-privilege requirement.
var Permissions = map[string]Permission{
	// --- no credentials: network probes and unauthenticated endpoints -------
	"certs": {Unauthenticated: true,
		Summary:   "Opens a TLS connection to the port and reads the presented certificate. No credential.",
		NotNeeded: "No account, no API access. The chain is not validated (the expiry is read even from an untrusted chain), so no trust store changes either."},
	"tls": {Unauthenticated: true,
		Summary:   "TLS handshake against the target. No credential.",
		NotNeeded: "No account and no server-side configuration."},
	"tcp": {Unauthenticated: true,
		Summary:   "Opens a TCP connection (optionally TLS) and optionally reads the banner. No credential.",
		NotNeeded: "Nothing is sent to the service beyond the connection itself."},
	"dns": {Unauthenticated: true,
		Summary:   "Sends DNS queries to the configured resolvers. No credential.",
		NotNeeded: "No zone transfer (AXFR) and no write: only ordinary lookups."},
	"ntp": {Unauthenticated: true,
		Summary:   "One SNTP request per server. No credential.",
		NotNeeded: "The local clock is never set — checkfleet reports the offset, it does not correct it."},
	"stream": {Unauthenticated: true,
		Summary:   "HTTP GET of the HLS/DASH manifest, and of the variant playlists it references.",
		NotNeeded: "No publishing credential: the check reads the delivery side, never the ingest side."},
	"ingest": {Unauthenticated: true,
		Summary:   "RTMP or SRT handshake only — it stops before publishing a stream.",
		NotNeeded: "No stream key and no publish permission: the check answers 'can a streamer connect', not 'can it publish'."},
	"grpc": {Unauthenticated: true,
		Summary:   "Calls the standard gRPC Health Checking service over HTTP/2.",
		NotNeeded: "No access to any application RPC."},
	"http": {Unauthenticated: true,
		Summary:   "Issues the configured request (GET by default) to the URL.",
		NotNeeded: "Whatever the endpoint itself requires is the operator's choice; checkfleet asks for no privilege of its own."},
	"smtp": {Unauthenticated: true,
		Summary:   "Connects, reads the greeting, sends EHLO and optionally negotiates STARTTLS.",
		NotNeeded: "It never authenticates and never sends mail — no mailbox, no relay permission."},
	"cassandra": {Unauthenticated: true,
		Summary:   "Speaks the CQL native handshake (OPTIONS/STARTUP) and stops at READY or AUTHENTICATE.",
		NotNeeded: "No credential and no keyspace access: reaching the AUTHENTICATE frame already proves the node accepts CQL."},
	"memcached": {Unauthenticated: true,
		Summary:   "Sends the text-protocol `stats` command.",
		NotNeeded: "No key is read or written. If SASL is enabled the check needs an account that may run `stats`."},
	"keycloak": {Unauthenticated: true,
		Summary:   "Reads the health endpoint and the public OIDC discovery document of each realm (`/realms/<realm>/.well-known/openid-configuration`).",
		NotNeeded: "No admin API access, no client secret, no service account: both endpoints are public by design."},
	"vault": {Unauthenticated: true,
		Summary:        "Reads `/v1/sys/seal-status` and `/v1/sys/health`, which Vault serves unauthenticated.",
		NotNeeded:      "No token is required. One may be configured, and if set it is sent, but the two endpoints used need no policy — no secret engine, no read of any path.",
		NeedsJudgement: true},
	"nats": {Unauthenticated: true,
		Summary:   "Reads the HTTP monitoring endpoints `/varz` and `/jsz?meta=1` on the monitoring port (8222 by default).",
		NotNeeded: "No NATS client credential: the monitoring port is separate from the client port, and no subject is subscribed or published."},
	"patroni": {Unauthenticated: true,
		Summary:        "GETs the Patroni REST API `/cluster` endpoint.",
		NotNeeded:      "No database credential, and none of Patroni's write endpoints (switchover, restart, reinitialise) is ever called.",
		NeedsJudgement: true},
	"haproxy": {Unauthenticated: true,
		Summary:        "GETs the stats page in CSV form (`/stats;csv`), which must be enabled in haproxy.cfg.",
		NotNeeded:      "No admin access to the stats socket: the check reads, it never enables or disables a server.",
		NeedsJudgement: true},

	// --- credentialed --------------------------------------------------------
	"postgres": {
		Summary: "Connects to one database and reads server statistics: `pg_is_in_recovery()`, `pg_database`, `pg_stat_activity`, `pg_replication_slots`, `pg_stat_replication` and `max_connections`. The built-in `pg_monitor` role covers all of it. Without it the check still connects, but a non-superuser sees the replication views empty — so the replica-lag and inactive-slot findings would report nothing wrong on a primary that has a problem.",
		Statements: []string{
			"CREATE ROLE checkfleet LOGIN PASSWORD '<from your secret store>';",
			"GRANT pg_monitor TO checkfleet;",
			"GRANT CONNECT ON DATABASE postgres TO checkfleet;",
		},
		NotNeeded: "No SELECT on any user table, no schema access, no SUPERUSER, no write of any kind. `pg_monitor` is read-only by construction.",
		// Measured, not assumed: on a standalone with no replication the check
		// passes with no grant at all, because pg_database and pg_stat_activity
		// are world-readable and the two replication views are empty. pg_monitor
		// is what makes those views *populated* for a non-superuser on a primary
		// with replicas — which is exactly where the check has something to say.
		// Grant it, or the replication findings silently report nothing wrong.
		NeedsJudgement: true},
	"mysql": {
		Summary: "Runs `SELECT VERSION()`, `SELECT @@global.read_only`, `SHOW GLOBAL STATUS`, `SHOW GLOBAL VARIABLES` and, on a replica, `SHOW REPLICA STATUS`.",
		Statements: []string{
			"CREATE USER 'checkfleet'@'%' IDENTIFIED BY '<from your secret store>';",
			"GRANT PROCESS, REPLICATION CLIENT ON *.* TO 'checkfleet'@'%';",
		},
		NotNeeded: "No SELECT on any schema, no SUPER, no RELOAD, no write. `REPLICATION CLIENT` grants the status view only — not the ability to change replication."},
	"mongodb": {
		Summary: "Runs the `serverStatus` and `replSetGetStatus` admin commands. The built-in `clusterMonitor` role covers both.",
		Statements: []string{
			`db.getSiblingDB("admin").createUser({user:"checkfleet", pwd:"<from your secret store>", roles:[{role:"clusterMonitor", db:"admin"}]})`,
		},
		NotNeeded: "No `read` on any application database, no `clusterManager`, no write. `clusterMonitor` cannot see document data."},
	"redis": {
		Summary: "Sends `INFO`. On Redis 6+ an ACL user restricted to that one command is enough.",
		Statements: []string{
			"ACL SETUSER checkfleet on >'<from your secret store>' ~ -@all +info",
		},
		NotNeeded: "No keyspace access (`~` matches no key), no `CONFIG`, no `KEYS`, no administrative or write command."},
	"kafka": {
		Summary: "Reads cluster metadata and, when consumer groups are configured, their committed offsets and lag.",
		Statements: []string{
			"kafka-acls --add --allow-principal User:checkfleet --operation Describe --cluster",
			"kafka-acls --add --allow-principal User:checkfleet --operation Describe --topic '*'",
			"kafka-acls --add --allow-principal User:checkfleet --operation Describe --group '*'",
		},
		NotNeeded: "No Read or Write on any topic: the check never consumes or produces a message, it reads metadata and offsets."},
	"elasticsearch": {
		Summary: "GETs `/_cluster/health` and `/_cat/allocation`. The built-in cluster privilege `monitor` covers both.",
		Statements: []string{
			`PUT /_security/role/checkfleet {"cluster":["monitor"]}`,
		},
		NotNeeded: "No index privilege at all: no document is read, and no index is created, written or deleted."},
	"clickhouse": {
		Summary: "Calls `/ping`, runs `SELECT version()` and reads `system.replicas`.",
		Statements: []string{
			"CREATE USER checkfleet IDENTIFIED BY '<from your secret store>';",
			"GRANT SELECT ON system.replicas TO checkfleet;",
		},
		NotNeeded: "No access to any user database or table, and no `SYSTEM` privileges (no restart, no replica manipulation)."},
	"rabbitmq": {
		Summary: "GETs the management API `/api/overview`, `/api/nodes` and `/api/queues`. The `monitoring` tag is exactly this level.",
		Statements: []string{
			"rabbitmqctl add_user checkfleet '<from your secret store>'",
			"rabbitmqctl set_user_tags checkfleet monitoring",
		},
		NotNeeded: "No `administrator` or `management` tag, and no configure/write/read permission on any vhost: queues are counted, never consumed from."},
	"consul": {
		Summary: "Reads `/v1/status/leader`, `/v1/status/peers`, `/v1/health/state/<state>` and any configured `/v1/kv/<key>`. With ACLs on, a token whose policy grants read on those.",
		Statements: []string{
			`# consul acl policy create -name checkfleet -rules -` + "\n" +
				`agent_prefix "" { policy = "read" }` + "\n" +
				`node_prefix  "" { policy = "read" }` + "\n" +
				`service_prefix "" { policy = "read" }` + "\n" +
				`key_prefix "<only the keys in kv_keys>" { policy = "read" }`,
		},
		NotNeeded:      "No `write` on anything, no `operator` policy, and key read scoped to the keys you actually configured rather than the whole KV tree.",
		NeedsJudgement: true},
	"etcd": {
		Summary: "Calls `/health`, `/v3/maintenance/status` and `/v3/cluster/member/list` over the JSON gateway. With auth enabled it first calls `/v3/auth/authenticate`.",
		Statements: []string{
			"etcdctl role add checkfleet",
			"etcdctl user add checkfleet",
			"etcdctl user grant-role checkfleet checkfleet",
		},
		NotNeeded:      "No key read or write: the endpoints used report cluster state, not data. Grant no key range to the role.",
		NeedsJudgement: true},
	"ldap": {
		Summary:        "Binds (anonymously, or with the configured account) and optionally runs one search under `base_dn`.",
		NotNeeded:      "No write of any kind, and read scoped to the subtree you point it at. When no sanity search is configured, the bind alone is enough and the account needs no read at all.",
		NeedsJudgement: true},
	"s3": {
		Summary: "HEADs the bucket and, when a sentinel object is configured, reads its metadata.",
		Statements: []string{
			`{"Effect":"Allow","Action":["s3:ListBucket"],"Resource":"arn:aws:s3:::<bucket>"}`,
			`{"Effect":"Allow","Action":["s3:GetObject"],"Resource":"arn:aws:s3:::<bucket>/<sentinel object>"}`,
		},
		NotNeeded: "No `s3:PutObject`, no `s3:DeleteObject`, and GetObject scoped to the single sentinel key rather than the bucket."},
}

// Perms returns the permission entry for a module and whether it exists.
func Perms(name string) (Permission, bool) {
	p, ok := Permissions[name]
	return p, ok
}
