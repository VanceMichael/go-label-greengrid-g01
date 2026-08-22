package tenant

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/pagination"
	"github.com/VanceMichael/greengrid/internal/storage/sqlite"
	"github.com/google/uuid"
	"strings"
	"time"
)

type Service struct{ store *sqlite.Store }

func NewService(store *sqlite.Store) *Service { return &Service{store: store} }
func (s *Service) Rename(ctx context.Context, tenantID, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("%w: tenant name", domain.ErrInvalid)
	}
	result, err := s.store.DB().ExecContext(ctx, `UPDATE tenants SET name=? WHERE id=? AND status='active'`, name, tenantID)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return domain.ErrAlreadyExists
		}
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return domain.ErrNotFound
	}
	return nil
}
func (s *Service) Suspend(ctx context.Context, tenantID, actorID, requestID string) error {
	return s.store.WithTx(ctx, func(tx *sql.Tx) error {
		var status string
		if err := tx.QueryRowContext(ctx, `SELECT status FROM tenants WHERE id=?`, tenantID).Scan(&status); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		if status != "active" {
			return domain.ErrState
		}
		if _, err := tx.ExecContext(ctx, `UPDATE tenants SET status='suspended' WHERE id=? AND status='active'`, tenantID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE users SET active=0 WHERE tenant_id=?`, tenantID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE sessions SET revoked=1 WHERE user_id IN (SELECT id FROM users WHERE tenant_id=?)`, tenantID); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT INTO audit_events(id,tenant_id,actor_id,aggregate_type,aggregate_id,action,result,request_id,details,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, uuid.NewString(), tenantID, actorID, "tenant", tenantID, "suspend", "success", requestID, "tenant and identities suspended", time.Now().UTC().Format(time.RFC3339Nano))
		return err
	})
}
func (s *Service) Reactivate(ctx context.Context, tenantID, actorID, requestID string) error {
	return s.store.WithTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE tenants SET status='active' WHERE id=? AND status='suspended'`, tenantID)
		if err != nil {
			return err
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			return domain.ErrConflict
		}
		_, err = tx.Exec(`INSERT INTO audit_events(id,tenant_id,actor_id,aggregate_type,aggregate_id,action,result,request_id,details,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, uuid.NewString(), tenantID, actorID, "tenant", tenantID, "reactivate", "success", requestID, "tenant reactivated; identities remain explicitly inactive", time.Now().UTC().Format(time.RFC3339Nano))
		return err
	})
}

type Member struct {
	ID, Email, DisplayName string
	Role                   domain.Role
	Active                 bool
	CreatedAt              time.Time
}

func (s *Service) Members(ctx context.Context, tenantID string, page pagination.Page) (pagination.Result[Member], error) {
	if err := page.Validate(); err != nil {
		return pagination.Result[Member]{}, err
	}
	var total int
	if err := s.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE tenant_id=?`, tenantID).Scan(&total); err != nil {
		return pagination.Result[Member]{}, err
	}
	rows, err := s.store.DB().QueryContext(ctx, `SELECT id,email,display_name,role,active,created_at FROM users WHERE tenant_id=? ORDER BY email LIMIT ? OFFSET ?`, tenantID, page.Limit, page.Offset)
	if err != nil {
		return pagination.Result[Member]{}, err
	}
	defer rows.Close()
	var out []Member
	for rows.Next() {
		var m Member
		var active int
		var created string
		if err := rows.Scan(&m.ID, &m.Email, &m.DisplayName, &m.Role, &active, &created); err != nil {
			return pagination.Result[Member]{}, err
		}
		m.Active = active != 0
		m.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		out = append(out, m)
	}
	return pagination.Result[Member]{Items: out, Meta: pagination.Meta{Total: total, Limit: page.Limit, Offset: page.Offset}}, rows.Err()
}
func (s *Service) ChangeRole(ctx context.Context, tenantID, actorID, userID string, role domain.Role, requestID string) error {
	if !role.Valid() {
		return fmt.Errorf("%w: role", domain.ErrInvalid)
	}
	return s.store.WithTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE users SET role=? WHERE id=? AND tenant_id=? AND active=1`, role, userID, tenantID)
		if err != nil {
			return err
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			return domain.ErrNotFound
		}
		_, err = tx.Exec(`INSERT INTO audit_events(id,tenant_id,actor_id,aggregate_type,aggregate_id,action,result,request_id,details,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, uuid.NewString(), tenantID, actorID, "user", userID, "role_change", "success", requestID, string(role), time.Now().UTC().Format(time.RFC3339Nano))
		return err
	})
}
