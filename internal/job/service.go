package job

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/storage/sqlite"
	"github.com/google/uuid"
)

type Service struct{ store *sqlite.Store }

func NewService(store *sqlite.Store) *Service { return &Service{store: store} }

func (s *Service) Submit(ctx context.Context, tenantID, reservationID, artifactVersionID, name string, gpu int, actorID, requestID string) (domain.Job, error) {
	if tenantID == "" || reservationID == "" || strings.TrimSpace(name) == "" || gpu <= 0 {
		return domain.Job{}, fmt.Errorf("%w: job fields", domain.ErrInvalid)
	}
	now := time.Now().UTC()
	j := domain.Job{ID: uuid.NewString(), TenantID: tenantID, ReservationID: reservationID, ArtifactVersionID: artifactVersionID, Name: name, GPUCount: gpu, Status: domain.JobQueued, Version: 1, CreatedAt: now}
	err := s.store.WithTx(ctx, func(tx *sql.Tx) error {
		var reservationStatus string
		var reservedGPU int
		if err := tx.QueryRowContext(ctx, `SELECT status,gpu_count FROM reservations WHERE id=? AND tenant_id=?`, reservationID, tenantID).Scan(&reservationStatus, &reservedGPU); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrNotFound
			}
			return fmt.Errorf("read job reservation: %w", err)
		}
		if reservationStatus != string(domain.ReservationActive) || gpu > reservedGPU {
			return fmt.Errorf("%w: reservation is not active for job", domain.ErrConflict)
		}
		if artifactVersionID != "" {
			var status string
			if err := tx.QueryRowContext(ctx, `SELECT status FROM artifact_versions WHERE id=?`, artifactVersionID).Scan(&status); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return domain.ErrNotFound
				}
				return err
			}
			if status == string(domain.ArtifactRetired) {
				return domain.ErrConflict
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO jobs(id,tenant_id,reservation_id,artifact_version_id,name,gpu_count,status,attempts,version,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, j.ID, tenantID, reservationID, nullable(artifactVersionID), name, gpu, j.Status, 0, 1, now.Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("insert job: %w", err)
		}
		if err := audit(tx, tenantID, actorID, "job", j.ID, "submit", requestID, "queued"); err != nil {
			return err
		}
		if err := outbox(tx, tenantID, "job.queued", j.ID, `{"status":"queued"}`); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return domain.Job{}, err
	}
	return j, nil
}

func (s *Service) Claim(ctx context.Context, workerID string, now time.Time) (domain.Job, error) {
	if workerID == "" {
		return domain.Job{}, fmt.Errorf("%w: worker id", domain.ErrInvalid)
	}
	var job domain.Job
	err := s.store.WithTx(ctx, func(tx *sql.Tx) error {
		var created string
		var artifact sql.NullString
		err := tx.QueryRowContext(ctx, `SELECT id,tenant_id,reservation_id,artifact_version_id,name,gpu_count,status,attempts,version,created_at FROM jobs WHERE status='queued' ORDER BY created_at,id LIMIT 1`).Scan(&job.ID, &job.TenantID, &job.ReservationID, &artifact, &job.Name, &job.GPUCount, &job.Status, &job.Attempts, &job.Version, &created)
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("find queued job: %w", err)
		}
		job.ArtifactVersionID = artifact.String
		job.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE jobs SET status='claimed',attempts=attempts+1,version=version+1 WHERE id=? AND status='queued'`, job.ID)
		if err != nil {
			return fmt.Errorf("claim job: %w", err)
		}
		changed, _ := result.RowsAffected()
		if changed != 1 {
			return domain.ErrConflict
		}
		leaseUntil := now.UTC().Add(2 * time.Minute).Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx, `INSERT INTO leases(resource_type,resource_id,owner,expires_at) VALUES('job',?,?,?) ON CONFLICT(resource_type,resource_id) DO UPDATE SET owner=excluded.owner,expires_at=excluded.expires_at`, job.ID, workerID, leaseUntil); err != nil {
			return fmt.Errorf("claim lease: %w", err)
		}
		job.Status = domain.JobClaimed
		job.Attempts++
		job.Version++
		return nil
	})
	return job, err
}

func (s *Service) Start(ctx context.Context, workerID, jobID string, expectedVersion int64) error {
	return s.store.WithTx(ctx, func(tx *sql.Tx) error {
		var owner string
		var expires string
		if err := tx.QueryRowContext(ctx, `SELECT owner,expires_at FROM leases WHERE resource_type='job' AND resource_id=?`, jobID).Scan(&owner, &expires); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrLeaseHeld
			}
			return err
		}
		if owner != workerID {
			return domain.ErrForbidden
		}
		if t, err := time.Parse(time.RFC3339Nano, expires); err != nil || !time.Now().UTC().Before(t) {
			return domain.ErrLeaseHeld
		}
		result, err := tx.ExecContext(ctx, `UPDATE jobs SET status='running',version=version+1,started_at=? WHERE id=? AND status='claimed' AND version=?`, time.Now().UTC().Format(time.RFC3339Nano), jobID, expectedVersion)
		if err != nil {
			return err
		}
		changed, _ := result.RowsAffected()
		if changed != 1 {
			return domain.ErrConflict
		}
		return nil
	})
}

func (s *Service) Finish(ctx context.Context, workerID, jobID string, expectedVersion int64, success bool, message, requestID string) error {
	return s.store.WithTx(ctx, func(tx *sql.Tx) error {
		var tenantID, status, owner string
		var attemptNo int
		if err := tx.QueryRowContext(ctx, `SELECT tenant_id,status,attempts FROM jobs WHERE id=?`, jobID).Scan(&tenantID, &status, &attemptNo); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		if err := tx.QueryRowContext(ctx, `SELECT owner FROM leases WHERE resource_type='job' AND resource_id=?`, jobID).Scan(&owner); err != nil {
			return err
		}
		if owner != workerID {
			return domain.ErrForbidden
		}
		from := domain.JobStatus(status)
		to := domain.JobFailed
		if success {
			to = domain.JobSucceeded
		}
		if err := domain.JobTransition(from, to); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE jobs SET status=?,version=version+1,finished_at=? WHERE id=? AND version=? AND status=?`, to, time.Now().UTC().Format(time.RFC3339Nano), jobID, expectedVersion, status)
		if err != nil {
			return err
		}
		changed, _ := result.RowsAffected()
		if changed != 1 {
			return domain.ErrConflict
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM leases WHERE resource_type='job' AND resource_id=? AND owner=?`, jobID, workerID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO job_attempts(id,job_id,attempt_no,worker_id,status,error_message,started_at,finished_at) VALUES(?,?,?,?,?,?,?,?)`, uuid.NewString(), jobID, attemptNo, workerID, string(to), nullable(message), time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
		return audit(tx, tenantID, workerID, "job", jobID, "finish", requestID, string(to))
	})
}

func (s *Service) Cancel(ctx context.Context, tenantID, actorID, jobID string, expectedVersion int64, requestID string) error {
	return s.store.WithTx(ctx, func(tx *sql.Tx) error {
		var status string
		if err := tx.QueryRowContext(ctx, `SELECT status FROM jobs WHERE id=? AND tenant_id=?`, jobID, tenantID).Scan(&status); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		if err := domain.JobTransition(domain.JobStatus(status), domain.JobCancelled); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE jobs SET status='cancelled',version=version+1,finished_at=? WHERE id=? AND tenant_id=? AND version=?`, time.Now().UTC().Format(time.RFC3339Nano), jobID, tenantID, expectedVersion)
		if err != nil {
			return err
		}
		changed, _ := result.RowsAffected()
		if changed != 1 {
			return domain.ErrConflict
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM leases WHERE resource_type='job' AND resource_id=?`, jobID); err != nil {
			return err
		}
		return audit(tx, tenantID, actorID, "job", jobID, "cancel", requestID, "cancelled")
	})
}

func (s *Service) ReleaseLease(ctx context.Context, workerID, jobID string) error {
	result, err := s.store.DB().ExecContext(ctx, `DELETE FROM leases WHERE resource_type='job' AND resource_id=? AND owner=?`, jobID, workerID)
	if err != nil {
		return fmt.Errorf("release lease: %w", err)
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Service) RequeueExpired(ctx context.Context, now time.Time) (int, error) {
	result, err := s.store.DB().ExecContext(ctx, `UPDATE jobs SET status='queued',version=version+1 WHERE status IN ('claimed','running') AND id IN (SELECT resource_id FROM leases WHERE resource_type='job' AND expires_at<=?)`, now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

func (s *Service) Get(ctx context.Context, tenantID, id string) (domain.Job, error) {
	var j domain.Job
	var artifact sql.NullString
	var created, started, finished sql.NullString
	err := s.store.DB().QueryRowContext(ctx, `SELECT id,tenant_id,reservation_id,artifact_version_id,name,gpu_count,status,attempts,version,created_at,started_at,finished_at FROM jobs WHERE id=? AND tenant_id=?`, id, tenantID).Scan(&j.ID, &j.TenantID, &j.ReservationID, &artifact, &j.Name, &j.GPUCount, &j.Status, &j.Attempts, &j.Version, &created, &started, &finished)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Job{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Job{}, err
	}
	j.ArtifactVersionID = artifact.String
	j.CreatedAt, _ = time.Parse(time.RFC3339Nano, created.String)
	if started.Valid {
		t, _ := time.Parse(time.RFC3339Nano, started.String)
		j.StartedAt = &t
	}
	if finished.Valid {
		t, _ := time.Parse(time.RFC3339Nano, finished.String)
		j.FinishedAt = &t
	}
	return j, nil
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}
func audit(tx *sql.Tx, tenantID, actorID, typ, id, action, requestID, details string) error {
	_, err := tx.Exec(`INSERT INTO audit_events(id,tenant_id,actor_id,aggregate_type,aggregate_id,action,result,request_id,details,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, uuid.NewString(), tenantID, actorID, typ, id, action, "success", requestID, details, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
func outbox(tx *sql.Tx, tenantID, kind, id, payload string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := tx.Exec(`INSERT INTO outbox_events(id,tenant_id,kind,aggregate_id,payload,status,attempts,next_attempt_at,created_at) VALUES(?,?,?,?,?,'pending',0,?,?)`, uuid.NewString(), tenantID, kind, id, payload, now, now)
	return err
}
