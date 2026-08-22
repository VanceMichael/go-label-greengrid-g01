package reservation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/VanceMichael/greengrid/internal/cluster"
	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/storage/sqlite"
	"github.com/google/uuid"
)

type Service struct {
	store   *sqlite.Store
	cluster *cluster.Service
}

func NewService(store *sqlite.Store, clusters *cluster.Service) *Service {
	return &Service{store: store, cluster: clusters}
}

func (s *Service) Request(ctx context.Context, tenantID, userID, clusterID string, gpu int, starts, ends time.Time, requestID string) (domain.Reservation, error) {
	if tenantID == "" || userID == "" || clusterID == "" || gpu <= 0 || !ends.After(starts) {
		return domain.Reservation{}, fmt.Errorf("%w: reservation request", domain.ErrInvalid)
	}
	r := domain.Reservation{ID: uuid.NewString(), TenantID: tenantID, ClusterID: clusterID, RequestedBy: userID, GPUCount: gpu, StartsAt: starts.UTC(), EndsAt: ends.UTC(), Status: domain.ReservationRequested, Version: 1, CreatedAt: time.Now().UTC()}
	err := s.store.WithTx(ctx, func(tx *sql.Tx) error {
		var owner string
		if err := tx.QueryRowContext(ctx, `SELECT tenant_id FROM clusters WHERE id=?`, clusterID).Scan(&owner); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrNotFound
			}
			return fmt.Errorf("read reservation cluster: %w", err)
		}
		if owner != tenantID {
			return domain.ErrForbidden
		}
		var overlap int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM reservations WHERE cluster_id=? AND status IN ('requested','approved','active') AND starts_at < ? AND ends_at > ?`, clusterID, r.EndsAt.Format(time.RFC3339Nano), r.StartsAt.Format(time.RFC3339Nano)).Scan(&overlap); err != nil {
			return fmt.Errorf("check reservation overlap: %w", err)
		}
		if overlap > 0 {
			return domain.ErrConflict
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO reservations(id,tenant_id,cluster_id,requested_by,gpu_count,starts_at,ends_at,status,version,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, r.ID, r.TenantID, r.ClusterID, r.RequestedBy, r.GPUCount, r.StartsAt.Format(time.RFC3339Nano), r.EndsAt.Format(time.RFC3339Nano), r.Status, 1, r.CreatedAt.Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("insert reservation: %w", err)
		}
		if err := addAudit(tx, tenantID, userID, "reservation", r.ID, "request", requestID, "requested"); err != nil {
			return fmt.Errorf("audit request: %w", err)
		}
		return nil
	})
	if err != nil {
		return domain.Reservation{}, err
	}
	return r, nil
}

func (s *Service) Approve(ctx context.Context, tenantID, approverID, reservationID string, expectedVersion int64, requestID string) error {
	return s.store.WithTx(ctx, func(tx *sql.Tx) error {
		var status string
		var clusterID string
		var gpu int
		if err := tx.QueryRowContext(ctx, `SELECT status,cluster_id,gpu_count FROM reservations WHERE id=? AND tenant_id=?`, reservationID, tenantID).Scan(&status, &clusterID, &gpu); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrNotFound
			}
			return fmt.Errorf("read reservation approval: %w", err)
		}
		if err := domain.ReservationTransition(domain.ReservationStatus(status), domain.ReservationApproved); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE reservations SET status='approved',version=version+1 WHERE id=? AND tenant_id=? AND version=? AND status='requested'`, reservationID, tenantID, expectedVersion); err != nil {
			return fmt.Errorf("approve reservation: %w", err)
		}
		var changed int64
		changed, _ = rowsChanged(tx)
		if changed != 1 {
			return domain.ErrConflict
		}
		if err := s.cluster.ReserveCapacity(tx, clusterID, gpu); err != nil {
			return err
		}
		if err := addAudit(tx, tenantID, approverID, "reservation", reservationID, "approve", requestID, "approved"); err != nil {
			return fmt.Errorf("audit approval: %w", err)
		}
		return nil
	})
}

func (s *Service) Activate(ctx context.Context, tenantID, reservationID string, expectedVersion int64, requestID string) error {
	return s.transition(ctx, tenantID, reservationID, domain.ReservationActive, expectedVersion, requestID)
}

func (s *Service) Cancel(ctx context.Context, tenantID, actorID, reservationID string, expectedVersion int64, requestID string) error {
	return s.store.WithTx(ctx, func(tx *sql.Tx) error {
		var status string
		var clusterID string
		var gpu int
		if err := tx.QueryRowContext(ctx, `SELECT status,cluster_id,gpu_count FROM reservations WHERE id=? AND tenant_id=?`, reservationID, tenantID).Scan(&status, &clusterID, &gpu); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrNotFound
			}
			return fmt.Errorf("read reservation cancellation: %w", err)
		}
		from := domain.ReservationStatus(status)
		if err := domain.ReservationTransition(from, domain.ReservationCancelled); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE reservations SET status='cancelled',version=version+1 WHERE id=? AND tenant_id=? AND version=?`, reservationID, tenantID, expectedVersion)
		if err != nil {
			return fmt.Errorf("cancel reservation: %w", err)
		}
		changed, _ := result.RowsAffected()
		if changed != 1 {
			return domain.ErrConflict
		}
		if from == domain.ReservationApproved || from == domain.ReservationActive {
			if err := s.cluster.ReleaseCapacity(tx, clusterID, gpu); err != nil {
				return err
			}
		}
		return addAudit(tx, tenantID, actorID, "reservation", reservationID, "cancel", requestID, "cancelled")
	})
}

func (s *Service) Release(ctx context.Context, tenantID, actorID, reservationID string, expectedVersion int64, requestID string) error {
	return s.store.WithTx(ctx, func(tx *sql.Tx) error {
		var status, clusterID string
		var gpu int
		if err := tx.QueryRowContext(ctx, `SELECT status,cluster_id,gpu_count FROM reservations WHERE id=? AND tenant_id=?`, reservationID, tenantID).Scan(&status, &clusterID, &gpu); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		if err := domain.ReservationTransition(domain.ReservationStatus(status), domain.ReservationReleased); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE reservations SET status='released',version=version+1 WHERE id=? AND tenant_id=? AND version=? AND status='active'`, reservationID, tenantID, expectedVersion)
		if err != nil {
			return fmt.Errorf("release reservation: %w", err)
		}
		changed, _ := result.RowsAffected()
		if changed != 1 {
			return domain.ErrConflict
		}
		if err := s.cluster.ReleaseCapacity(tx, clusterID, gpu); err != nil {
			return err
		}
		return addAudit(tx, tenantID, actorID, "reservation", reservationID, "release", requestID, "released")
	})
}

func (s *Service) transition(ctx context.Context, tenantID, id string, to domain.ReservationStatus, expected int64, requestID string) error {
	return s.store.WithTx(ctx, func(tx *sql.Tx) error {
		var from string
		if err := tx.QueryRowContext(ctx, `SELECT status FROM reservations WHERE id=? AND tenant_id=?`, id, tenantID).Scan(&from); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		if err := domain.ReservationTransition(domain.ReservationStatus(from), to); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE reservations SET status=?,version=version+1 WHERE id=? AND tenant_id=? AND version=?`, to, id, tenantID, expected)
		if err != nil {
			return fmt.Errorf("transition reservation: %w", err)
		}
		changed, _ := result.RowsAffected()
		if changed != 1 {
			return domain.ErrConflict
		}
		return addAudit(tx, tenantID, "system", "reservation", id, string(to), requestID, string(to))
	})
}

func (s *Service) Get(ctx context.Context, tenantID, id string) (domain.Reservation, error) {
	var r domain.Reservation
	var starts, ends, created string
	err := s.store.DB().QueryRowContext(ctx, `SELECT id,tenant_id,cluster_id,requested_by,gpu_count,starts_at,ends_at,status,version,created_at FROM reservations WHERE id=? AND tenant_id=?`, id, tenantID).Scan(&r.ID, &r.TenantID, &r.ClusterID, &r.RequestedBy, &r.GPUCount, &starts, &ends, &r.Status, &r.Version, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Reservation{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Reservation{}, err
	}
	r.StartsAt, err = time.Parse(time.RFC3339Nano, starts)
	if err != nil {
		return domain.Reservation{}, err
	}
	r.EndsAt, err = time.Parse(time.RFC3339Nano, ends)
	if err != nil {
		return domain.Reservation{}, err
	}
	r.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	return r, err
}

func (s *Service) ListOverlapping(ctx context.Context, tenantID, clusterID string, starts, ends time.Time) ([]domain.Reservation, error) {
	rows, err := s.store.DB().QueryContext(ctx, `SELECT id,tenant_id,cluster_id,requested_by,gpu_count,starts_at,ends_at,status,version,created_at FROM reservations WHERE tenant_id=? AND cluster_id=? AND status IN ('requested','approved','active') AND starts_at < ? AND ends_at > ? ORDER BY starts_at`, tenantID, clusterID, ends.UTC().Format(time.RFC3339Nano), starts.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.Reservation
	for rows.Next() {
		var r domain.Reservation
		var a, b, c string
		if err := rows.Scan(&r.ID, &r.TenantID, &r.ClusterID, &r.RequestedBy, &r.GPUCount, &a, &b, &r.Status, &r.Version, &c); err != nil {
			return nil, err
		}
		r.StartsAt, err = time.Parse(time.RFC3339Nano, a)
		if err != nil {
			return nil, err
		}
		r.EndsAt, err = time.Parse(time.RFC3339Nano, b)
		if err != nil {
			return nil, err
		}
		r.CreatedAt, err = time.Parse(time.RFC3339Nano, c)
		if err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func addAudit(tx *sql.Tx, tenantID, actorID, aggregateType, aggregateID, action, requestID, details string) error {
	_, err := tx.Exec(`INSERT INTO audit_events(id,tenant_id,actor_id,aggregate_type,aggregate_id,action,result,request_id,details,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, uuid.NewString(), tenantID, actorID, aggregateType, aggregateID, action, "success", requestID, details, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func rowsChanged(tx *sql.Tx) (int64, error) {
	var count int64
	err := tx.QueryRow(`SELECT changes()`).Scan(&count)
	return count, err
}
