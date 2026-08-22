package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

const schemaVersion = 1

func (s *Store) Migrate(ctx context.Context) error {
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
			return fmt.Errorf("create migration table: %w", err)
		}
		var current int
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&current); err != nil {
			return fmt.Errorf("read migration version: %w", err)
		}
		if current >= schemaVersion {
			return nil
		}
		if _, err := tx.ExecContext(ctx, schemaSQL); err != nil {
			return fmt.Errorf("apply schema v%d: %w", schemaVersion, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES(?, datetime('now'))`, schemaVersion); err != nil {
			return fmt.Errorf("record migration: %w", err)
		}
		return nil
	})
}

const schemaSQL = `
CREATE TABLE IF NOT EXISTS tenants (
 id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, status TEXT NOT NULL, created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS users (
 id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL REFERENCES tenants(id), email TEXT NOT NULL UNIQUE,
 display_name TEXT NOT NULL, role TEXT NOT NULL, active INTEGER NOT NULL, created_at TEXT NOT NULL,
 password_hash TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
 id TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES users(id), token_hash TEXT NOT NULL UNIQUE,
 expires_at TEXT NOT NULL, revoked INTEGER NOT NULL, created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS clusters (
 id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL REFERENCES tenants(id), name TEXT NOT NULL,
 region TEXT NOT NULL, capacity_gpu INTEGER NOT NULL, reserved_gpu INTEGER NOT NULL,
 version INTEGER NOT NULL, status TEXT NOT NULL, created_at TEXT NOT NULL, UNIQUE(tenant_id,name)
);
CREATE TABLE IF NOT EXISTS nodes (
 id TEXT PRIMARY KEY, cluster_id TEXT NOT NULL REFERENCES clusters(id), name TEXT NOT NULL,
 gpu_capacity INTEGER NOT NULL, gpu_reserved INTEGER NOT NULL, status TEXT NOT NULL,
 version INTEGER NOT NULL, created_at TEXT NOT NULL, UNIQUE(cluster_id,name)
);
CREATE TABLE IF NOT EXISTS reservations (
 id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL REFERENCES tenants(id), cluster_id TEXT NOT NULL REFERENCES clusters(id),
 requested_by TEXT NOT NULL REFERENCES users(id), gpu_count INTEGER NOT NULL, starts_at TEXT NOT NULL,
 ends_at TEXT NOT NULL, status TEXT NOT NULL, version INTEGER NOT NULL, created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_reservation_window ON reservations(cluster_id,status,starts_at,ends_at);
CREATE TABLE IF NOT EXISTS jobs (
 id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL REFERENCES tenants(id), reservation_id TEXT NOT NULL REFERENCES reservations(id),
 artifact_version_id TEXT, name TEXT NOT NULL, gpu_count INTEGER NOT NULL, status TEXT NOT NULL,
 attempts INTEGER NOT NULL, version INTEGER NOT NULL, created_at TEXT NOT NULL, started_at TEXT, finished_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_jobs_queue ON jobs(status,created_at);
CREATE TABLE IF NOT EXISTS job_attempts (
 id TEXT PRIMARY KEY, job_id TEXT NOT NULL REFERENCES jobs(id), attempt_no INTEGER NOT NULL,
 worker_id TEXT NOT NULL, status TEXT NOT NULL, error_message TEXT, started_at TEXT NOT NULL, finished_at TEXT,
 UNIQUE(job_id,attempt_no)
);
CREATE TABLE IF NOT EXISTS leases (
 resource_type TEXT NOT NULL, resource_id TEXT NOT NULL, owner TEXT NOT NULL, expires_at TEXT NOT NULL,
 PRIMARY KEY(resource_type,resource_id)
);
CREATE TABLE IF NOT EXISTS telemetry_readings (
 id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL REFERENCES tenants(id), node_id TEXT NOT NULL REFERENCES nodes(id),
 sequence INTEGER NOT NULL, measured_at TEXT NOT NULL, power_watts REAL NOT NULL, renewable_share REAL NOT NULL,
 UNIQUE(node_id,sequence)
);
CREATE INDEX IF NOT EXISTS idx_telemetry_window ON telemetry_readings(node_id,measured_at);
CREATE TABLE IF NOT EXISTS carbon_reports (
 id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL REFERENCES tenants(id), cluster_id TEXT NOT NULL REFERENCES clusters(id),
 window_start TEXT NOT NULL, window_end TEXT NOT NULL, energy_kwh REAL NOT NULL, carbon_grams REAL NOT NULL,
 renewable_share REAL NOT NULL, status TEXT NOT NULL, version INTEGER NOT NULL, created_at TEXT NOT NULL,
 UNIQUE(cluster_id,window_start,window_end)
);
CREATE TABLE IF NOT EXISTS artifacts (
 id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL REFERENCES tenants(id), name TEXT NOT NULL,
 active_version_id TEXT, status TEXT NOT NULL, version INTEGER NOT NULL, created_at TEXT NOT NULL,
 UNIQUE(tenant_id,name)
);
CREATE TABLE IF NOT EXISTS artifact_versions (
 id TEXT PRIMARY KEY, artifact_id TEXT NOT NULL REFERENCES artifacts(id), digest TEXT NOT NULL,
 status TEXT NOT NULL, size_bytes INTEGER NOT NULL, version INTEGER NOT NULL, created_at TEXT NOT NULL,
 UNIQUE(artifact_id,digest)
);
CREATE TABLE IF NOT EXISTS outbox_events (
 id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL REFERENCES tenants(id), kind TEXT NOT NULL,
 aggregate_id TEXT NOT NULL, payload TEXT NOT NULL, status TEXT NOT NULL, attempts INTEGER NOT NULL,
 lease_owner TEXT, lease_until TEXT, next_attempt_at TEXT NOT NULL, created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_outbox_ready ON outbox_events(status,next_attempt_at);
CREATE TABLE IF NOT EXISTS audit_events (
 id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL REFERENCES tenants(id), actor_id TEXT, aggregate_type TEXT NOT NULL,
 aggregate_id TEXT NOT NULL, action TEXT NOT NULL, result TEXT NOT NULL, request_id TEXT NOT NULL,
 details TEXT NOT NULL, created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_aggregate ON audit_events(aggregate_type,aggregate_id,created_at);
`
