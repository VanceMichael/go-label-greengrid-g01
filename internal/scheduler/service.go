package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/storage/sqlite"
	"github.com/google/uuid"
	"time"
)

type Service struct{ store *sqlite.Store }
type BatchResult struct {
	JobID  string
	Status string
	Err    error
}

func NewService(store *sqlite.Store) *Service { return &Service{store: store} }
func (s *Service) CancelBatch(ctx context.Context, tenantID, actorID, requestID string, ids []string) []BatchResult {
	out := make([]BatchResult, 0, len(ids))
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			out = append(out, BatchResult{JobID: id, Status: "cancelled", Err: err})
			continue
		}
		err := s.cancelOne(ctx, tenantID, actorID, requestID, id)
		status := "cancelled"
		if err != nil {
			status = "failed"
		}
		out = append(out, BatchResult{JobID: id, Status: status, Err: err})
	}
	return out
}
func (s *Service) cancelOne(ctx context.Context, tenantID, actorID, requestID, id string) error {
	return s.store.WithTx(ctx, func(tx *sql.Tx) error {
		var status string
		if err := tx.QueryRowContext(ctx, `SELECT status FROM jobs WHERE id=? AND tenant_id=?`, id, tenantID).Scan(&status); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		if err := domain.JobTransition(domain.JobStatus(status), domain.JobCancelled); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE jobs SET status='cancelled',version=version+1,finished_at=? WHERE id=? AND tenant_id=? AND status IN ('queued','claimed','running')`, time.Now().UTC().Format(time.RFC3339Nano), id, tenantID)
		if err != nil {
			return err
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			return domain.ErrConflict
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM leases WHERE resource_type='job' AND resource_id=?`, id); err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO audit_events(id,tenant_id,actor_id,aggregate_type,aggregate_id,action,result,request_id,details,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, uuid.NewString(), tenantID, actorID, "job", id, "batch_cancel", "success", requestID, "cancelled")
		return err
	})
}
func (s *Service) QueueDepth(ctx context.Context, tenantID string) (int, error) {
	var count int
	err := s.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM jobs WHERE tenant_id=? AND status='queued'`, tenantID).Scan(&count)
	return count, err
}
func (s *Service) NextQueued(ctx context.Context, tenantID string) (domain.Job, error) {
	var j domain.Job
	var created string
	err := s.store.DB().QueryRowContext(ctx, `SELECT id,tenant_id,reservation_id,COALESCE(artifact_version_id,''),name,gpu_count,status,attempts,version,created_at FROM jobs WHERE tenant_id=? AND status='queued' ORDER BY created_at,id LIMIT 1`, tenantID).Scan(&j.ID, &j.TenantID, &j.ReservationID, &j.ArtifactVersionID, &j.Name, &j.GPUCount, &j.Status, &j.Attempts, &j.Version, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Job{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Job{}, err
	}
	j.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	return j, err
}
func EnsureTenant(tenantID string) error {
	if tenantID == "" {
		return fmt.Errorf("%w: tenant", domain.ErrInvalid)
	}
	return nil
}
