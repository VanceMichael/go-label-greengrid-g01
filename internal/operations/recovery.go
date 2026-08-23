package operations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/storage/sqlite"
	"time"
)

type RecoveryAction struct {
	ResourceType, ResourceID, PreviousStatus, NextStatus, Reason string
	Applied                                                      bool
}
type RecoveryService struct{ store *sqlite.Store }

func NewRecoveryService(store *sqlite.Store) *RecoveryService { return &RecoveryService{store: store} }
func (r *RecoveryService) RecoverJobs(ctx context.Context, tenantID, actorID, requestID string, now time.Time) ([]RecoveryAction, error) {
	rows, err := r.store.DB().QueryContext(ctx, `SELECT j.id,j.status FROM jobs j JOIN leases l ON l.resource_type='job' AND l.resource_id=j.id WHERE j.tenant_id=? AND l.expires_at<=? AND j.status IN ('claimed','running') ORDER BY j.created_at,j.id`, tenantID, now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	var candidates []struct{ id, status string }
	for rows.Next() {
		var id, status string
		if err := rows.Scan(&id, &status); err != nil {
			_ = rows.Close()
			return nil, err
		}
		candidates = append(candidates, struct{ id, status string }{id: id, status: status})
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	var actions []RecoveryAction
	for _, candidate := range candidates {
		action, err := r.recoverOne(ctx, tenantID, actorID, requestID, candidate.id, candidate.status, now)
		if err != nil {
			// ErrLeaseHeld means the lease was renewed between the scan and
			// the recovery transaction; the job is no longer expired, so
			// skip it rather than aborting the whole batch.
			if errors.Is(err, domain.ErrLeaseHeld) {
				action = RecoveryAction{ResourceType: "job", ResourceID: candidate.id, PreviousStatus: candidate.status, NextStatus: candidate.status, Reason: "lease renewed during recovery", Applied: false}
				actions = append(actions, action)
				continue
			}
			return nil, err
		}
		actions = append(actions, action)
	}
	return actions, rows.Err()
}
func (r *RecoveryService) recoverOne(ctx context.Context, tenantID, actorID, requestID, id, status string, now time.Time) (RecoveryAction, error) {
	action := RecoveryAction{ResourceType: "job", ResourceID: id, PreviousStatus: status, NextStatus: "queued", Reason: "lease expired"}
	err := r.store.WithTx(ctx, func(tx *sql.Tx) error {
		// Re-check expiry inside the recovery transaction. A worker that
		// renewed the lease between the scan and this statement pushed
		// expires_at past now; the live lease must be left in place and
		// the job must not be requeued. Signal skip via ErrLeaseHeld.
		var expires string
		switch err := tx.QueryRowContext(ctx, `SELECT expires_at FROM leases WHERE resource_type='job' AND resource_id=?`, id).Scan(&expires); {
		case errors.Is(err, sql.ErrNoRows):
			return domain.ErrNotFound
		case err != nil:
			return err
		}
		t, err := time.Parse(time.RFC3339Nano, expires)
		if err != nil {
			return err
		}
		if t.After(now) {
			return domain.ErrLeaseHeld
		}
		result, err := tx.ExecContext(ctx, `UPDATE jobs SET status='queued',version=version+1 WHERE id=? AND tenant_id=? AND status IN ('claimed','running')`, id, tenantID)
		if err != nil {
			return err
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			return domain.ErrConflict
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM leases WHERE resource_type='job' AND resource_id=? AND expires_at<=?`, id, now.UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO audit_events(id,tenant_id,actor_id,aggregate_type,aggregate_id,action,result,request_id,details,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, fmt.Sprintf("recovery-%d", time.Now().UnixNano()), tenantID, actorID, "job", id, "recover_lease", "success", requestID, "expired lease returned job to queue", time.Now().UTC().Format(time.RFC3339Nano))
		return err
	})
	action.Applied = err == nil
	return action, err
}
func (r *RecoveryService) RecoverOutbox(ctx context.Context, tenantID string, now time.Time) (int, error) {
	result, err := r.store.DB().ExecContext(ctx, `UPDATE outbox_events SET status='retry',lease_owner=NULL,lease_until=NULL,next_attempt_at=? WHERE tenant_id=? AND status='sending' AND lease_until<=?`, now.UTC().Format(time.RFC3339Nano), tenantID, now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}
func (r *RecoveryService) CheckJob(ctx context.Context, tenantID, id string) (RecoveryAction, error) {
	var status string
	err := r.store.DB().QueryRowContext(ctx, `SELECT status FROM jobs WHERE id=? AND tenant_id=?`, id, tenantID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return RecoveryAction{}, domain.ErrNotFound
	}
	if err != nil {
		return RecoveryAction{}, err
	}
	var lease string
	err = r.store.DB().QueryRowContext(ctx, `SELECT owner FROM leases WHERE resource_type='job' AND resource_id=?`, id).Scan(&lease)
	if errors.Is(err, sql.ErrNoRows) {
		return RecoveryAction{ResourceType: "job", ResourceID: id, PreviousStatus: status, NextStatus: status, Reason: "no lease", Applied: false}, nil
	}
	if err != nil {
		return RecoveryAction{}, err
	}
	return RecoveryAction{ResourceType: "job", ResourceID: id, PreviousStatus: status, NextStatus: status, Reason: "lease owned by " + lease, Applied: false}, nil
}
func (r *RecoveryService) Drain(ctx context.Context, tenantID string, deadline time.Time) error {
	for time.Now().Before(deadline) {
		var running int
		if err := r.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM jobs WHERE tenant_id=? AND status IN ('claimed','running')`, tenantID).Scan(&running); err != nil {
			return err
		}
		if running == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
	return domain.ErrConflict
}
