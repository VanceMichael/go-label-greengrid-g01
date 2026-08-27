package job

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/pagination"
	"github.com/VanceMichael/greengrid/internal/storage/sqlite"
	"time"
)

type Stats struct{ Queued, Claimed, Running, Succeeded, Failed, Cancelled int }
type Query struct{ store *sqlite.Store }

func NewQuery(store *sqlite.Store) *Query { return &Query{store: store} }
func (q *Query) Stats(ctx context.Context, tenantID string) (Stats, error) {
	var s Stats
	rows, err := q.store.DB().QueryContext(ctx, `SELECT status,COUNT(*) FROM jobs WHERE tenant_id=? GROUP BY status`, tenantID)
	if err != nil {
		return s, err
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return s, err
		}
		switch domain.JobStatus(status) {
		case domain.JobQueued:
			s.Queued = count
		case domain.JobClaimed:
			s.Claimed = count
		case domain.JobRunning:
			s.Running = count
		case domain.JobSucceeded:
			s.Succeeded = count
		case domain.JobFailed:
			s.Failed = count
		case domain.JobCancelled:
			s.Cancelled = count
		}
	}
	return s, rows.Err()
}
func (q *Query) List(ctx context.Context, tenantID, status string, page pagination.Page) (pagination.Result[domain.Job], error) {
	if err := page.Validate(); err != nil {
		return pagination.Result[domain.Job]{}, err
	}
	where := "tenant_id=?"
	args := []any{tenantID}
	if status != "" {
		where += " AND status=?"
		args = append(args, status)
	}
	var total int
	if err := q.store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM jobs WHERE "+where, args...).Scan(&total); err != nil {
		return pagination.Result[domain.Job]{}, err
	}
	args = append(args, page.Limit, page.Offset)
	rows, err := q.store.DB().QueryContext(ctx, "SELECT id,tenant_id,reservation_id,COALESCE(artifact_version_id,''),name,gpu_count,status,attempts,version,created_at,started_at,finished_at FROM jobs WHERE "+where+" ORDER BY created_at DESC,id DESC LIMIT ? OFFSET ?", args...)
	if err != nil {
		return pagination.Result[domain.Job]{}, err
	}
	defer rows.Close()
	var out []domain.Job
	for rows.Next() {
		var j domain.Job
		var c, st, fi sql.NullString
		if err := rows.Scan(&j.ID, &j.TenantID, &j.ReservationID, &j.ArtifactVersionID, &j.Name, &j.GPUCount, &j.Status, &j.Attempts, &j.Version, &c, &st, &fi); err != nil {
			return pagination.Result[domain.Job]{}, err
		}
		j.CreatedAt, _ = time.Parse(time.RFC3339Nano, c.String)
		if st.Valid {
			v, _ := time.Parse(time.RFC3339Nano, st.String)
			j.StartedAt = &v
		}
		if fi.Valid {
			v, _ := time.Parse(time.RFC3339Nano, fi.String)
			j.FinishedAt = &v
		}
		out = append(out, j)
	}
	return pagination.Result[domain.Job]{Items: out, Meta: pagination.Meta{Total: total, Limit: page.Limit, Offset: page.Offset}}, rows.Err()
}
func (q *Query) RecoverExpiredLeases(ctx context.Context, now time.Time) (int, error) {
	result, err := q.store.DB().ExecContext(ctx, `UPDATE jobs SET status='queued',version=version+1 WHERE status IN ('claimed','running') AND id IN (SELECT resource_id FROM leases WHERE resource_type='job' AND expires_at<=?)`, now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, fmt.Errorf("recover jobs: %w", err)
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}
func (q *Query) Find(ctx context.Context, id string) (domain.Job, error) {
	var j domain.Job
	var c string
	err := q.store.DB().QueryRowContext(ctx, `SELECT id,tenant_id,reservation_id,COALESCE(artifact_version_id,''),name,gpu_count,status,attempts,version,created_at FROM jobs WHERE id=?`, id).Scan(&j.ID, &j.TenantID, &j.ReservationID, &j.ArtifactVersionID, &j.Name, &j.GPUCount, &j.Status, &j.Attempts, &j.Version, &c)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Job{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Job{}, err
	}
	j.CreatedAt, err = time.Parse(time.RFC3339Nano, c)
	return j, err
}
