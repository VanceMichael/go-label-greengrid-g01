package reservation

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

type Summary struct {
	ID, ClusterID    string
	GPUCount         int
	Status           domain.ReservationStatus
	StartsAt, EndsAt time.Time
	Version          int64
}
type Query struct{ store *sqlite.Store }

func NewQuery(store *sqlite.Store) *Query { return &Query{store: store} }
func (q *Query) List(ctx context.Context, tenantID, status string, page pagination.Page) (pagination.Result[Summary], error) {
	if err := page.Validate(); err != nil {
		return pagination.Result[Summary]{}, err
	}
	where := "tenant_id=?"
	args := []any{tenantID}
	if status != "" {
		where += " AND status=?"
		args = append(args, status)
	}
	var total int
	if err := q.store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM reservations WHERE "+where, args...).Scan(&total); err != nil {
		return pagination.Result[Summary]{}, err
	}
	args = append(args, page.Limit, page.Offset)
	rows, err := q.store.DB().QueryContext(ctx, "SELECT id,cluster_id,gpu_count,status,starts_at,ends_at,version FROM reservations WHERE "+where+" ORDER BY starts_at,id LIMIT ? OFFSET ?", args...)
	if err != nil {
		return pagination.Result[Summary]{}, err
	}
	defer rows.Close()
	var out []Summary
	for rows.Next() {
		var x Summary
		var start, end string
		if err := rows.Scan(&x.ID, &x.ClusterID, &x.GPUCount, &x.Status, &start, &end, &x.Version); err != nil {
			return pagination.Result[Summary]{}, err
		}
		x.StartsAt, err = time.Parse(time.RFC3339Nano, start)
		if err != nil {
			return pagination.Result[Summary]{}, err
		}
		x.EndsAt, err = time.Parse(time.RFC3339Nano, end)
		if err != nil {
			return pagination.Result[Summary]{}, err
		}
		out = append(out, x)
	}
	return pagination.Result[Summary]{Items: out, Meta: pagination.Meta{Total: total, Limit: page.Limit, Offset: page.Offset}}, rows.Err()
}
func (q *Query) ActiveAt(ctx context.Context, tenantID, clusterID string, at time.Time) ([]Summary, error) {
	rows, err := q.store.DB().QueryContext(ctx, `SELECT id,cluster_id,gpu_count,status,starts_at,ends_at,version FROM reservations WHERE tenant_id=? AND cluster_id=? AND status IN ('requested','approved','active') AND starts_at<=? AND ends_at>? ORDER BY starts_at`, tenantID, clusterID, at.UTC().Format(time.RFC3339Nano), at.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Summary
	for rows.Next() {
		var x Summary
		var a, b string
		if err := rows.Scan(&x.ID, &x.ClusterID, &x.GPUCount, &x.Status, &a, &b, &x.Version); err != nil {
			return nil, err
		}
		x.StartsAt, _ = time.Parse(time.RFC3339Nano, a)
		x.EndsAt, _ = time.Parse(time.RFC3339Nano, b)
		out = append(out, x)
	}
	return out, rows.Err()
}
func ValidateWindow(start, end time.Time) error {
	if start.IsZero() || end.IsZero() || !end.After(start) {
		return fmt.Errorf("%w: reservation window", domain.ErrInvalid)
	}
	if end.Sub(start) > 30*24*time.Hour {
		return fmt.Errorf("%w: reservation too long", domain.ErrInvalid)
	}
	return nil
}

var _ = errors.Is
var _ = sql.ErrNoRows
