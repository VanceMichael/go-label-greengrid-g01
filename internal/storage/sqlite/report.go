package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/VanceMichael/greengrid/internal/domain"
	"time"
)

type Snapshot struct{ Tenants, Users, Clusters, Nodes, Reservations, Jobs, Readings, Reports, Artifacts, Outbox int }

func (s *Store) Snapshot(ctx context.Context) (Snapshot, error) {
	var out Snapshot
	entries := []struct {
		name   string
		target *int
	}{{"tenants", &out.Tenants}, {"users", &out.Users}, {"clusters", &out.Clusters}, {"nodes", &out.Nodes}, {"reservations", &out.Reservations}, {"jobs", &out.Jobs}, {"telemetry_readings", &out.Readings}, {"carbon_reports", &out.Reports}, {"artifacts", &out.Artifacts}, {"outbox_events", &out.Outbox}}
	for _, entry := range entries {
		if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+entry.name).Scan(entry.target); err != nil {
			return Snapshot{}, err
		}
	}
	return out, nil
}
func (s *Store) Vacuum(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(PASSIVE)"); err != nil {
		return fmt.Errorf("checkpoint: %w", err)
	}
	return nil
}
func (s *Store) RunReadOnly(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
func (s *Store) CountTenant(ctx context.Context, tenantID, table string) (int, error) {
	allowed := map[string]bool{"users": true, "clusters": true, "nodes": true, "reservations": true, "jobs": true, "artifacts": true, "carbon_reports": true}
	if !allowed[table] {
		return 0, domain.ErrInvalid
	}
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE tenant_id=?", tenantID).Scan(&n)
	return n, err
}
func (s *Store) PurgeRevokedSessions(ctx context.Context, before time.Time) (int, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE revoked=1 OR expires_at<?`, before.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}
func (s *Store) HasTenant(ctx context.Context, tenantID string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tenants WHERE id=?`, tenantID).Scan(&n)
	return n == 1, err
}
func (s *Store) TenantStatus(ctx context.Context, tenantID string) (string, error) {
	var status string
	err := s.db.QueryRowContext(ctx, `SELECT status FROM tenants WHERE id=?`, tenantID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	return status, err
}
