package identity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/identity"
	"github.com/VanceMichael/greengrid/internal/testsupport"
)

func identityServices(t *testing.T) testsupport.Services {
	t.Helper()
	s, err := testsupport.Open(t.TempDir() + "/identity.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Store.Close() })
	return s
}

func TestCreateTenantAndUserStoresRoles(t *testing.T) {
	s := identityServices(t)
	ctx := context.Background()
	tenant, err := s.Identity.CreateTenant(ctx, "wind-and-light")
	if err != nil {
		t.Fatal(err)
	}
	user, err := s.Identity.CreateUser(ctx, tenant, "ops@example.com", "Operator", "password", domain.RoleClusterOps)
	if err != nil {
		t.Fatal(err)
	}
	if user.TenantID != tenant || user.Role != domain.RoleClusterOps || !user.Active {
		t.Fatalf("unexpected user: %+v", user)
	}
	var count int
	if err := s.Store.DB().QueryRow(`SELECT COUNT(*) FROM users WHERE tenant_id=?`, tenant).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("user count=%d", count)
	}
}

func TestAuthenticateCreatesOpaqueExpiringSession(t *testing.T) {
	s := identityServices(t)
	ctx := context.Background()
	tenant, _ := s.Identity.CreateTenant(ctx, "training-tenant")
	_, _ = s.Identity.CreateUser(ctx, tenant, "user@example.com", "User", "correct", domain.RoleScheduler)
	session, user, err := s.Identity.Authenticate(ctx, "USER@example.com", "correct")
	if err != nil {
		t.Fatal(err)
	}
	if session.TokenHash == "" || session.ID == "" || !session.ExpiresAt.After(time.Now()) {
		t.Fatalf("bad session: %+v", session)
	}
	if user.Email != "user@example.com" {
		t.Fatal(user.Email)
	}
	got, stored, err := s.Identity.AuthenticateToken(ctx, session.TokenHash)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != user.ID || stored.ID != session.ID {
		t.Fatalf("auth mismatch got=%+v stored=%+v", got, stored)
	}
}

func TestAuthenticationRejectsWrongExpiredAndRevoked(t *testing.T) {
	s := identityServices(t)
	ctx := context.Background()
	tenant, _ := s.Identity.CreateTenant(ctx, "tenant")
	u, _ := s.Identity.CreateUser(ctx, tenant, "user@example.com", "User", "correct", domain.RoleTenantAdmin)
	if _, _, err := s.Identity.Authenticate(ctx, "user@example.com", "wrong"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("wrong password err=%v", err)
	}
	if _, _, err := s.Identity.Authenticate(ctx, "missing@example.com", "correct"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("missing user err=%v", err)
	}
	session, _, err := s.Identity.Authenticate(ctx, "user@example.com", "correct")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Identity.RevokeSession(ctx, session.ID, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Identity.AuthenticateToken(ctx, session.TokenHash); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("revoked token err=%v", err)
	}
	short := identity.NewService(s.Store, time.Millisecond)
	fresh, _, err := short.Authenticate(ctx, "user@example.com", "correct")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, _, err := short.AuthenticateToken(ctx, fresh.TokenHash); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expired token err=%v", err)
	}
}

func TestDeactivateUserIsAtomicAndRevokesAllSessions(t *testing.T) {
	s := identityServices(t)
	ctx := context.Background()
	tenant, _ := s.Identity.CreateTenant(ctx, "tenant")
	actor, _ := s.Identity.CreateUser(ctx, tenant, "admin@example.com", "Admin", "secret", domain.RolePlatformAdmin)
	target, _ := s.Identity.CreateUser(ctx, tenant, "target@example.com", "Target", "secret", domain.RoleScheduler)
	one, _, _ := s.Identity.Authenticate(ctx, "target@example.com", "secret")
	two, _, _ := s.Identity.Authenticate(ctx, "target@example.com", "secret")
	if err := s.Identity.DeactivateUser(ctx, actor.ID, target.ID, "request-1"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Identity.AuthenticateToken(ctx, one.TokenHash); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("session one remains: %v", err)
	}
	if _, _, err := s.Identity.AuthenticateToken(ctx, two.TokenHash); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("session two remains: %v", err)
	}
	var active int
	_ = s.Store.DB().QueryRow(`SELECT active FROM users WHERE id=?`, target.ID).Scan(&active)
	if active != 0 {
		t.Fatalf("active=%d", active)
	}
	var audits int
	_ = s.Store.DB().QueryRow(`SELECT COUNT(*) FROM audit_events WHERE aggregate_id=?`, target.ID).Scan(&audits)
	if audits != 1 {
		t.Fatalf("audits=%d", audits)
	}
}

func TestDeactivateUnknownAndAlreadyInactive(t *testing.T) {
	s := identityServices(t)
	ctx := context.Background()
	tenant, _ := s.Identity.CreateTenant(ctx, "tenant")
	actor, _ := s.Identity.CreateUser(ctx, tenant, "admin@example.com", "Admin", "secret", domain.RolePlatformAdmin)
	if err := s.Identity.DeactivateUser(ctx, actor.ID, "missing", "r"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing err=%v", err)
	}
	target, _ := s.Identity.CreateUser(ctx, tenant, "target@example.com", "Target", "secret", domain.RoleScheduler)
	if err := s.Identity.DeactivateUser(ctx, actor.ID, target.ID, "r"); err != nil {
		t.Fatal(err)
	}
	if err := s.Identity.DeactivateUser(ctx, actor.ID, target.ID, "r2"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("inactive err=%v", err)
	}
}
