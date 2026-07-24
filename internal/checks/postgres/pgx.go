package postgres

import (
	"context"
	"os"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	"github.com/jackc/pgx/v5"
)

// pgxConnect is the default collector: a real PostgreSQL connection via pgx.
// The password, if any, comes from the target's PasswordEnv — never the config.
func pgxConnect(ctx context.Context, t engine.PostgresTarget) (collector, error) {
	cfg, err := pgx.ParseConfig(t.DSN)
	if err != nil {
		return nil, err
	}
	if t.PasswordEnv != "" {
		if pw := os.Getenv(t.PasswordEnv); pw != "" {
			cfg.Password = pw
		}
	}
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &pgxCollector{conn: conn}, nil
}

type pgxCollector struct {
	conn *pgx.Conn
}

func (p *pgxCollector) Close(ctx context.Context) { _ = p.conn.Close(ctx) }

func (p *pgxCollector) Collect(ctx context.Context) (metrics, error) {
	var m metrics
	if err := p.conn.QueryRow(ctx, `SELECT pg_is_in_recovery()`).Scan(&m.InRecovery); err != nil {
		return m, err
	}
	if err := p.conn.QueryRow(ctx, `SELECT COALESCE(max(age(datfrozenxid)), 0) FROM pg_database`).Scan(&m.WraparoundAge); err != nil {
		return m, err
	}
	if err := p.conn.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity`).Scan(&m.Connections); err != nil {
		return m, err
	}
	if err := p.conn.QueryRow(ctx, `SELECT current_setting('max_connections')::int`).Scan(&m.MaxConnections); err != nil {
		return m, err
	}

	slots, err := p.collectSlots(ctx)
	if err != nil {
		return m, err
	}
	m.InactiveSlots = slots

	if !m.InRecovery {
		replicas, err := p.collectReplicas(ctx)
		if err != nil {
			return m, err
		}
		m.Replicas = replicas
	}
	return m, nil
}

func (p *pgxCollector) collectSlots(ctx context.Context) ([]slot, error) {
	const q = `
SELECT slot_name,
       COALESCE(pg_wal_lsn_diff(
           CASE WHEN pg_is_in_recovery() THEN pg_last_wal_replay_lsn() ELSE pg_current_wal_lsn() END,
           restart_lsn), 0)::bigint
FROM pg_replication_slots
WHERE active = false`
	rows, err := p.conn.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var slots []slot
	for rows.Next() {
		var s slot
		if err := rows.Scan(&s.Name, &s.RetainedBytes); err != nil {
			return nil, err
		}
		slots = append(slots, s)
	}
	return slots, rows.Err()
}

func (p *pgxCollector) collectReplicas(ctx context.Context) ([]replica, error) {
	const q = `
SELECT COALESCE(host(client_addr), 'local'),
       state,
       COALESCE(pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn), 0)::bigint
FROM pg_stat_replication`
	rows, err := p.conn.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var replicas []replica
	for rows.Next() {
		var r replica
		if err := rows.Scan(&r.Client, &r.State, &r.LagBytes); err != nil {
			return nil, err
		}
		replicas = append(replicas, r)
	}
	return replicas, rows.Err()
}
