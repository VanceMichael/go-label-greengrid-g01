package maintenance

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/VanceMichael/greengrid/internal/cluster"
	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/storage/sqlite"
)

type Service struct{ store *sqlite.Store }

func NewService(store *sqlite.Store) *Service { return &Service{store: store} }
func (s *Service) Begin(ctx context.Context, tenantID, actorID, nodeID, requestID string) error {
	clusters := cluster.NewService(s.store)
	// The ready->maintenance flip and the maintenance audit event are committed
	// in one transaction so that an audit-write failure rolls the node back to
	// ready. Without this, a failed audit would leave the node in maintenance
	// while capacity is still reserved, and Begin could never be retried (the
	// retry guard requires status='ready').
	return s.store.WithTx(ctx, func(tx *sql.Tx) error {
		var clusterID, status string
		if err := tx.QueryRowContext(ctx, `SELECT n.cluster_id,n.status FROM nodes n JOIN clusters c ON c.id=n.cluster_id WHERE n.id=? AND c.tenant_id=?`, nodeID, tenantID).Scan(&clusterID, &status); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		if status != "ready" {
			return domain.ErrState
		}
		var active int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM reservations WHERE cluster_id=? AND status IN ('approved','active')`, clusterID).Scan(&active); err != nil {
			return err
		}
		if active > 0 {
			return fmt.Errorf("%w: active reservations", domain.ErrConflict)
		}
		if err := clusters.SetMaintenanceTx(tx, tenantID, nodeID); err != nil {
			return err
		}
		if err := sqlite.ExecAudit(tx, tenantID, actorID, "node", nodeID, "maintenance", "success", requestID, "maintenance started"); err != nil {
			return fmt.Errorf("audit maintenance: %w", err)
		}
		return nil
	})
}
func (s *Service) Complete(ctx context.Context, tenantID, actorID, nodeID, requestID string) error {
	return s.store.WithTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE nodes SET status='ready',version=version+1 WHERE id=? AND status='maintenance' AND cluster_id IN (SELECT id FROM clusters WHERE tenant_id=?)`, nodeID, tenantID)
		if err != nil {
			return err
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			return domain.ErrConflict
		}
		if err := sqlite.ExecAudit(tx, tenantID, actorID, "node", nodeID, "maintenance_complete", "success", requestID, "maintenance completed"); err != nil {
			return fmt.Errorf("audit maintenance completion: %w", err)
		}
		return nil
	})
}
func (s *Service) SetOffline(ctx context.Context, tenantID, actorID, nodeID, requestID string) error {
	return s.store.WithTx(ctx, func(tx *sql.Tx) error {
		var status string
		if err := tx.QueryRowContext(ctx, `SELECT n.status FROM nodes n JOIN clusters c ON c.id=n.cluster_id WHERE n.id=? AND c.tenant_id=?`, nodeID, tenantID).Scan(&status); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		if status == "offline" {
			return nil
		}
		if status == "ready" {
			return domain.ErrState
		}
		if _, err := tx.ExecContext(ctx, `UPDATE nodes SET status='offline',version=version+1 WHERE id=?`, nodeID); err != nil {
			return err
		}
		if err := sqlite.ExecAudit(tx, tenantID, actorID, "node", nodeID, "offline", "success", requestID, "node offline"); err != nil {
			return fmt.Errorf("audit offline: %w", err)
		}
		return nil
	})
}
