package outbox

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/storage/sqlite"
	"github.com/google/uuid"
)

type Service struct {
	store       *sqlite.Store
	maxAttempts int
}

func NewService(store *sqlite.Store, maxAttempts int) *Service {
	if maxAttempts < 1 {
		maxAttempts = 5
	}
	return &Service{store: store, maxAttempts: maxAttempts}
}

func (s *Service) Claim(ctx context.Context, owner string, now time.Time) (domain.OutboxEvent, error) {
	var event domain.OutboxEvent
	err := s.store.WithTx(ctx, func(tx *sql.Tx) error {
		var lease sql.NullString
		var until sql.NullString
		var next, created string
		err := tx.QueryRowContext(ctx, `SELECT id,tenant_id,kind,aggregate_id,payload,status,attempts,lease_owner,lease_until,next_attempt_at,created_at FROM outbox_events WHERE status IN ('pending','retry') AND next_attempt_at<=? ORDER BY created_at,id LIMIT 1`, now.UTC().Format(time.RFC3339Nano)).Scan(&event.ID, &event.TenantID, &event.Kind, &event.AggregateID, &event.Payload, &event.Status, &event.Attempts, &lease, &until, &next, &created)
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		event.LeaseOwner = lease.String
		if until.Valid {
			t, _ := time.Parse(time.RFC3339Nano, until.String)
			event.LeaseUntil = &t
		}
		event.NextAttemptAt, _ = time.Parse(time.RFC3339Nano, next)
		if event.NextAttemptAt.IsZero() {
			event.NextAttemptAt = now
		}
		if _, err := tx.ExecContext(ctx, `UPDATE outbox_events SET status='sending',lease_owner=?,lease_until=? WHERE id=? AND status IN ('pending','retry')`, owner, now.UTC().Add(time.Minute).Format(time.RFC3339Nano), event.ID); err != nil {
			return err
		}
		event.Status = "sending"
		event.LeaseOwner = owner
		t := now.UTC().Add(time.Minute)
		event.LeaseUntil = &t
		_ = created
		return nil
	})
	return event, err
}

func (s *Service) Finish(ctx context.Context, owner, eventID string, sendErr error) error {
	return s.store.WithTx(ctx, func(tx *sql.Tx) error {
		var tenantID string
		var attempts int
		var status, leaseOwner string
		if err := tx.QueryRowContext(ctx, `SELECT tenant_id,attempts,status,COALESCE(lease_owner,'') FROM outbox_events WHERE id=?`, eventID).Scan(&tenantID, &attempts, &status, &leaseOwner); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		if leaseOwner != owner {
			return domain.ErrForbidden
		}
		nextStatus := "sent"
		if sendErr != nil {
			attempts++
			if attempts >= s.maxAttempts {
				nextStatus = "failed"
			} else {
				nextStatus = "retry"
			}
		}
		var result sql.Result
		var err error
		if sendErr != nil && nextStatus == "retry" {
			result, err = tx.ExecContext(ctx, `UPDATE outbox_events SET status=?,attempts=?,lease_owner=NULL,lease_until=NULL,next_attempt_at=? WHERE id=? AND status='sending' AND lease_owner=?`, nextStatus, attempts, time.Now().UTC().Add(time.Duration(attempts)*time.Second).Format(time.RFC3339Nano), eventID, owner)
		} else {
			result, err = tx.ExecContext(ctx, `UPDATE outbox_events SET status=?,attempts=?,lease_owner=NULL,lease_until=NULL WHERE id=? AND status='sending' AND lease_owner=?`, nextStatus, attempts, eventID, owner)
		}
		if err != nil {
			return err
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			return domain.ErrConflict
		}
		details := "sent"
		if sendErr != nil {
			details = sendErr.Error()
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO audit_events(id,tenant_id,actor_id,aggregate_type,aggregate_id,action,result,request_id,details,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, uuid.NewString(), tenantID, owner, "outbox", eventID, "finish", nextStatus, eventID, details, time.Now().UTC().Format(time.RFC3339Nano))
		return err
	})
}

func (s *Service) RecoverExpired(ctx context.Context, now time.Time) (int, error) {
	result, err := s.store.DB().ExecContext(ctx, `UPDATE outbox_events SET status='retry',lease_owner=NULL,lease_until=NULL,next_attempt_at=? WHERE status='sending' AND lease_until<=?`, now.UTC().Format(time.RFC3339Nano), now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

func (s *Service) Count(ctx context.Context, status string) (int, error) {
	var n int
	err := s.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM outbox_events WHERE status=?`, status).Scan(&n)
	return n, err
}

func IsTerminal(event domain.OutboxEvent) bool {
	return event.Status == "sent" || event.Status == "failed"
}
func classify(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("outbox delivery: %w", err)
}
