package mysql

import (
	"context"
	"database/sql"
	"strconv"
	"strings"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	_ "github.com/go-sql-driver/mysql" // registers the "mysql" driver
)

type sqlCollector struct{ db *sql.DB }

// driverConnect opens a pooled connection to a target. The DSN should carry the
// password via ${ENV} interpolation (applied at config load), never inline.
func driverConnect(ctx context.Context, t engine.MySQLTarget) (collector, error) {
	db, err := sql.Open("mysql", t.DSN)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(2)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &sqlCollector{db: db}, nil
}

func (c *sqlCollector) Close() { _ = c.db.Close() }

func (c *sqlCollector) Collect(ctx context.Context) (metrics, error) {
	var m metrics

	if err := c.db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&m.Version); err != nil {
		return m, err
	}
	// read_only is best-effort: some managed flavours restrict @@global access.
	var ro sql.NullInt64
	if err := c.db.QueryRowContext(ctx, "SELECT @@global.read_only").Scan(&ro); err == nil {
		m.ReadOnly = ro.Valid && ro.Int64 == 1
	}

	m.Connections = c.statusInt(ctx, "Threads_connected")
	m.MaxConnections = c.variableInt(ctx, "max_connections")

	replica, err := c.replicaStatus(ctx)
	if err != nil {
		return m, err
	}
	m.Replica = replica
	return m, nil
}

// statusInt reads one SHOW GLOBAL STATUS counter (0 if absent).
func (c *sqlCollector) statusInt(ctx context.Context, name string) int {
	var k string
	var v sql.NullString
	if err := c.db.QueryRowContext(ctx, "SHOW GLOBAL STATUS LIKE ?", name).Scan(&k, &v); err != nil {
		return 0
	}
	n, _ := strconv.Atoi(v.String)
	return n
}

// variableInt reads one SHOW GLOBAL VARIABLES value (0 if absent).
func (c *sqlCollector) variableInt(ctx context.Context, name string) int {
	var k string
	var v sql.NullString
	if err := c.db.QueryRowContext(ctx, "SHOW GLOBAL VARIABLES LIKE ?", name).Scan(&k, &v); err != nil {
		return 0
	}
	n, _ := strconv.Atoi(v.String)
	return n
}

// replicaStatus runs SHOW REPLICA STATUS (falling back to the legacy SHOW SLAVE
// STATUS) and maps the version-dependent columns. Returns nil when the server is
// not a replica (empty result set).
func (c *sqlCollector) replicaStatus(ctx context.Context) (*replicaStatus, error) {
	row, err := c.showStatusRow(ctx, "SHOW REPLICA STATUS")
	if err != nil {
		if row, err = c.showStatusRow(ctx, "SHOW SLAVE STATUS"); err != nil {
			return nil, err
		}
	}
	if row == nil {
		return nil, nil // not a replica
	}
	get := func(keys ...string) (string, bool) {
		for _, k := range keys {
			if v, ok := row[k]; ok {
				return v.String, v.Valid
			}
		}
		return "", false
	}
	rs := &replicaStatus{}
	io, _ := get("Replica_IO_Running", "Slave_IO_Running")
	sqlr, _ := get("Replica_SQL_Running", "Slave_SQL_Running")
	rs.IORunning = strings.EqualFold(io, "Yes")
	rs.SQLRunning = strings.EqualFold(sqlr, "Yes")
	if sec, valid := get("Seconds_Behind_Source", "Seconds_Behind_Master"); valid {
		rs.Replicating = true
		rs.SecondsBehind, _ = strconv.ParseInt(sec, 10, 64)
	}
	return rs, nil
}

// showStatusRow runs a SHOW ...STATUS query and returns its single row as a
// name→value map (nil if there are no rows). NULL columns have Valid=false.
func (c *sqlCollector) showStatusRow(ctx context.Context, query string) (map[string]sql.NullString, error) {
	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	if !rows.Next() {
		return nil, rows.Err()
	}
	vals := make([]sql.NullString, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return nil, err
	}
	out := make(map[string]sql.NullString, len(cols))
	for i, name := range cols {
		out[name] = vals[i]
	}
	return out, nil
}
