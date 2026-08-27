package audit

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/pagination"
	"github.com/VanceMichael/greengrid/internal/storage/sqlite"
	"github.com/google/uuid"
)

type Event struct {
	ID, TenantID, ActorID, AggregateType, AggregateID, Action, Result, RequestID, Details string
	CreatedAt                                                                             time.Time
}
type Service struct{ store *sqlite.Store }

func NewService(store *sqlite.Store) *Service { return &Service{store: store} }

func (s *Service) Append(ctx context.Context, tenantID, actorID, typ, aggregateID, action, result, requestID, details string) error {
	if tenantID == "" || typ == "" || aggregateID == "" || action == "" || requestID == "" {
		return fmt.Errorf("%w: audit fields", domain.ErrInvalid)
	}
	_, err := s.store.DB().ExecContext(ctx, `INSERT INTO audit_events(id,tenant_id,actor_id,aggregate_type,aggregate_id,action,result,request_id,details,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, uuid.NewString(), tenantID, actorID, typ, aggregateID, action, result, requestID, details, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Service) List(ctx context.Context, tenantID, typ, aggregateID string, page pagination.Page) (pagination.Result[Event], error) {
	if page.Limit <= 0 || page.Limit > 100 {
		return pagination.Result[Event]{}, fmt.Errorf("%w: page limit", domain.ErrInvalid)
	}
	args := []any{tenantID}
	where := "tenant_id=?"
	if typ != "" {
		where += " AND aggregate_type=?"
		args = append(args, typ)
	}
	if aggregateID != "" {
		where += " AND aggregate_id=?"
		args = append(args, aggregateID)
	}
	var total int
	if err := s.store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_events WHERE "+where, args...).Scan(&total); err != nil {
		return pagination.Result[Event]{}, err
	}
	args = append(args, page.Limit, page.Offset)
	rows, err := s.store.DB().QueryContext(ctx, "SELECT id,tenant_id,COALESCE(actor_id,''),aggregate_type,aggregate_id,action,result,request_id,details,created_at FROM audit_events WHERE "+where+" ORDER BY created_at DESC,id DESC LIMIT ? OFFSET ?", args...)
	if err != nil {
		return pagination.Result[Event]{}, err
	}
	defer rows.Close()
	var items []Event
	for rows.Next() {
		var e Event
		var created string
		if err := rows.Scan(&e.ID, &e.TenantID, &e.ActorID, &e.AggregateType, &e.AggregateID, &e.Action, &e.Result, &e.RequestID, &e.Details, &created); err != nil {
			return pagination.Result[Event]{}, err
		}
		e.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return pagination.Result[Event]{}, err
		}
		items = append(items, e)
	}
	return pagination.Result[Event]{Items: items, Meta: pagination.Meta{Total: total, Limit: page.Limit, Offset: page.Offset}}, rows.Err()
}

func (s *Service) Get(ctx context.Context, tenantID, id string) (Event, error) {
	var e Event
	var created string
	err := s.store.DB().QueryRowContext(ctx, `SELECT id,tenant_id,COALESCE(actor_id,''),aggregate_type,aggregate_id,action,result,request_id,details,created_at FROM audit_events WHERE id=? AND tenant_id=?`, id, tenantID).Scan(&e.ID, &e.TenantID, &e.ActorID, &e.AggregateType, &e.AggregateID, &e.Action, &e.Result, &e.RequestID, &e.Details, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return Event{}, domain.ErrNotFound
	}
	if err != nil {
		return Event{}, err
	}
	e.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	return e, err
}
