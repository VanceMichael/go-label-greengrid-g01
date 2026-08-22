package outbox

import (
	"context"
	"database/sql"
	"errors"
	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/pagination"
	"github.com/VanceMichael/greengrid/internal/storage/sqlite"
	"time"
)

type Query struct{ store *sqlite.Store }

func NewQuery(store *sqlite.Store) *Query { return &Query{store: store} }
func (q *Query) List(ctx context.Context, tenantID, status string, page pagination.Page) (pagination.Result[domain.OutboxEvent], error) {
	if err := page.Validate(); err != nil {
		return pagination.Result[domain.OutboxEvent]{}, err
	}
	where := "tenant_id=?"
	args := []any{tenantID}
	if status != "" {
		where += " AND status=?"
		args = append(args, status)
	}
	var total int
	if err := q.store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM outbox_events WHERE "+where, args...).Scan(&total); err != nil {
		return pagination.Result[domain.OutboxEvent]{}, err
	}
	rows, err := q.store.DB().QueryContext(ctx, "SELECT id,tenant_id,kind,aggregate_id,payload,status,attempts,COALESCE(lease_owner,''),lease_until,next_attempt_at FROM outbox_events WHERE "+where+" ORDER BY created_at,id LIMIT ? OFFSET ?", append(args, page.Limit, page.Offset)...)
	if err != nil {
		return pagination.Result[domain.OutboxEvent]{}, err
	}
	defer rows.Close()
	var out []domain.OutboxEvent
	for rows.Next() {
		var e domain.OutboxEvent
		var until, next string
		if err := rows.Scan(&e.ID, &e.TenantID, &e.Kind, &e.AggregateID, &e.Payload, &e.Status, &e.Attempts, &e.LeaseOwner, &until, &next); err != nil {
			return pagination.Result[domain.OutboxEvent]{}, err
		}
		if until != "" {
			t, _ := time.Parse(time.RFC3339Nano, until)
			e.LeaseUntil = &t
		}
		e.NextAttemptAt, _ = time.Parse(time.RFC3339Nano, next)
		out = append(out, e)
	}
	return pagination.Result[domain.OutboxEvent]{Items: out, Meta: pagination.Meta{Total: total, Limit: page.Limit, Offset: page.Offset}}, rows.Err()
}
func (q *Query) Ready(ctx context.Context, now time.Time) (int, error) {
	var count int
	err := q.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM outbox_events WHERE status IN ('pending','retry') AND next_attempt_at<=?`, now.UTC().Format(time.RFC3339Nano)).Scan(&count)
	return count, err
}
func (q *Query) Get(ctx context.Context, id string) (domain.OutboxEvent, error) {
	var e domain.OutboxEvent
	var until, next string
	err := q.store.DB().QueryRowContext(ctx, `SELECT id,tenant_id,kind,aggregate_id,payload,status,attempts,COALESCE(lease_owner,''),COALESCE(lease_until,''),next_attempt_at FROM outbox_events WHERE id=?`, id).Scan(&e.ID, &e.TenantID, &e.Kind, &e.AggregateID, &e.Payload, &e.Status, &e.Attempts, &e.LeaseOwner, &until, &next)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.OutboxEvent{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.OutboxEvent{}, err
	}
	if until != "" {
		t, _ := time.Parse(time.RFC3339Nano, until)
		e.LeaseUntil = &t
	}
	e.NextAttemptAt, _ = time.Parse(time.RFC3339Nano, next)
	return e, nil
}
