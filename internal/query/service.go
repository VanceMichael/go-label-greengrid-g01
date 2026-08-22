package query

import (
	"context"
	"database/sql"
	"errors"
	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/pagination"
	"github.com/VanceMichael/greengrid/internal/storage/sqlite"
	"time"
)

type Service struct{ store *sqlite.Store }

func NewService(store *sqlite.Store) *Service { return &Service{store: store} }
func (s *Service) Jobs(ctx context.Context, tenantID, status, clusterID string, page pagination.Page) (pagination.Result[domain.Job], error) {
	if err := page.Validate(); err != nil {
		return pagination.Result[domain.Job]{}, err
	}
	where := "j.tenant_id=?"
	args := []any{tenantID}
	if status != "" {
		where += " AND j.status=?"
		args = append(args, status)
	}
	if clusterID != "" {
		where += " AND r.cluster_id=?"
		args = append(args, clusterID)
	}
	var total int
	if err := s.store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM jobs j JOIN reservations r ON r.id=j.reservation_id WHERE "+where, args...).Scan(&total); err != nil {
		return pagination.Result[domain.Job]{}, err
	}
	args = append(args, page.Limit, page.Offset)
	rows, err := s.store.DB().QueryContext(ctx, "SELECT j.id,j.tenant_id,j.reservation_id,COALESCE(j.artifact_version_id,''),j.name,j.gpu_count,j.status,j.attempts,j.version,j.created_at,j.started_at,j.finished_at FROM jobs j JOIN reservations r ON r.id=j.reservation_id WHERE "+where+" ORDER BY j.created_at DESC,j.id DESC LIMIT ? OFFSET ?", args...)
	if err != nil {
		return pagination.Result[domain.Job]{}, err
	}
	defer rows.Close()
	var items []domain.Job
	for rows.Next() {
		var j domain.Job
		var created, started, finished sql.NullString
		if err := rows.Scan(&j.ID, &j.TenantID, &j.ReservationID, &j.ArtifactVersionID, &j.Name, &j.GPUCount, &j.Status, &j.Attempts, &j.Version, &created, &started, &finished); err != nil {
			return pagination.Result[domain.Job]{}, err
		}
		j.CreatedAt, _ = time.Parse(time.RFC3339Nano, created.String)
		if started.Valid {
			t, _ := time.Parse(time.RFC3339Nano, started.String)
			j.StartedAt = &t
		}
		if finished.Valid {
			t, _ := time.Parse(time.RFC3339Nano, finished.String)
			j.FinishedAt = &t
		}
		items = append(items, j)
	}
	return pagination.Result[domain.Job]{Items: items, Meta: pagination.Meta{Total: total, Limit: page.Limit, Offset: page.Offset}}, rows.Err()
}
func (s *Service) ReservationSummary(ctx context.Context, tenantID, id string) (map[string]any, error) {
	var status string
	var start, end string
	var gpu int
	err := s.store.DB().QueryRowContext(ctx, `SELECT status,starts_at,ends_at,gpu_count FROM reservations WHERE id=? AND tenant_id=?`, id, tenantID).Scan(&status, &start, &end, &gpu)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": id, "status": status, "gpu_count": gpu, "starts_at": start, "ends_at": end}, nil
}
