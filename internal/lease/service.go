package lease

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/storage/sqlite"
	"time"
)

type Lease struct {
	ResourceType, ResourceID, Owner string
	ExpiresAt                       time.Time
}
type Service struct{ store *sqlite.Store }

func NewService(store *sqlite.Store) *Service { return &Service{store: store} }
func (s *Service) Acquire(ctx context.Context, typ, id, owner string, now, ttl time.Time) (Lease, error) {
	if typ == "" || id == "" || owner == "" || !ttl.After(now) {
		return Lease{}, fmt.Errorf("%w: lease", domain.ErrInvalid)
	}
	var out Lease
	err := s.store.WithTx(ctx, func(tx *sql.Tx) error {
		var currentOwner, expires string
		err := tx.QueryRowContext(ctx, `SELECT owner,expires_at FROM leases WHERE resource_type=? AND resource_id=?`, typ, id).Scan(&currentOwner, &expires)
		if err == nil {
			t, _ := time.Parse(time.RFC3339Nano, expires)
			if t.After(now) {
				return domain.ErrLeaseHeld
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		out = Lease{ResourceType: typ, ResourceID: id, Owner: owner, ExpiresAt: ttl.UTC()}
		_, err = tx.ExecContext(ctx, `INSERT INTO leases(resource_type,resource_id,owner,expires_at) VALUES(?,?,?,?) ON CONFLICT(resource_type,resource_id) DO UPDATE SET owner=excluded.owner,expires_at=excluded.expires_at`, typ, id, owner, out.ExpiresAt.Format(time.RFC3339Nano))
		return err
	})
	return out, err
}
func (s *Service) Release(ctx context.Context, typ, id, owner string) error {
	result, err := s.store.DB().ExecContext(ctx, `DELETE FROM leases WHERE resource_type=? AND resource_id=? AND owner=?`, typ, id, owner)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return domain.ErrNotFound
	}
	return nil
}
func (s *Service) Renew(ctx context.Context, typ, id, owner string, now, ttl time.Time) error {
	result, err := s.store.DB().ExecContext(ctx, `UPDATE leases SET expires_at=? WHERE resource_type=? AND resource_id=? AND owner=? AND expires_at>?`, ttl.UTC().Format(time.RFC3339Nano), typ, id, owner, now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return domain.ErrLeaseHeld
	}
	return nil
}
func (s *Service) Recover(ctx context.Context, now time.Time) (int, error) {
	result, err := s.store.DB().ExecContext(ctx, `DELETE FROM leases WHERE expires_at<=?`, now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}
