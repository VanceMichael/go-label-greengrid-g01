package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/identity"
	"github.com/VanceMichael/greengrid/internal/storage/sqlite"
	"github.com/google/uuid"
)

type Supervisor struct {
	ID        string
	CreatedAt time.Time
}

// EnsureSupervisor is used only by an explicit deployment bootstrap command.
// It is deliberately idempotent and never inserts credentials or tenant data.
func EnsureSupervisor(ctx context.Context, store *sqlite.Store) (Supervisor, error) {
	var id, created string
	err := store.DB().QueryRowContext(ctx, `SELECT id,created_at FROM users WHERE role=? ORDER BY created_at LIMIT 1`, domain.RolePlatformAdmin).Scan(&id, &created)
	if err == nil {
		t, e := time.Parse(time.RFC3339Nano, created)
		return Supervisor{ID: id, CreatedAt: t}, e
	}
	if err != sql.ErrNoRows {
		return Supervisor{}, fmt.Errorf("find supervisor: %w", err)
	}
	return Supervisor{ID: uuid.NewString(), CreatedAt: time.Now().UTC()}, domain.ErrNotFound
}

func ProvisionSupervisor(ctx context.Context, store *sqlite.Store, tenantID, email, password string) (Supervisor, error) {
	if err := ctx.Err(); err != nil {
		return Supervisor{}, err
	}
	var id, created string
	err := store.DB().QueryRowContext(ctx, `SELECT id,created_at FROM users WHERE role=? ORDER BY created_at LIMIT 1`, domain.RolePlatformAdmin).Scan(&id, &created)
	if err == nil {
		t, parseErr := time.Parse(time.RFC3339Nano, created)
		return Supervisor{ID: id, CreatedAt: t}, parseErr
	}
	if err != sql.ErrNoRows {
		return Supervisor{}, fmt.Errorf("find supervisor for provision: %w", err)
	}
	users := identity.NewService(store, 24*time.Hour)
	user, err := users.CreateUser(ctx, tenantID, email, "GreenGrid Supervisor", password, domain.RolePlatformAdmin)
	if err != nil {
		return Supervisor{}, fmt.Errorf("provision supervisor: %w", err)
	}
	return Supervisor{ID: user.ID, CreatedAt: user.CreatedAt}, nil
}
