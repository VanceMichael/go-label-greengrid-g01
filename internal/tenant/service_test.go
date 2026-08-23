package tenant_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/tenant"
	"github.com/VanceMichael/greengrid/internal/testsupport"
)

func tenantServices(t *testing.T) (testsupport.Services, *tenant.Service) {
	t.Helper()
	s, err := testsupport.Open(t.TempDir() + "/tenant.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Store.Close() })
	return s, tenant.NewService(s.Store)
}

func TestSuspendIsAtomicAndRecordsAudit(t *testing.T) {
	s, svc := tenantServices(t)
	ctx := context.Background()
	tenantID, _ := s.Identity.CreateTenant(ctx, "compute-tenant")
	actor, _ := s.Identity.CreateUser(ctx, tenantID, "admin@example.com", "Admin", "secret", domain.RoleTenantAdmin)
	target, _ := s.Identity.CreateUser(ctx, tenantID, "ops@example.com", "Ops", "secret", domain.RoleClusterOps)
	session, _, _ := s.Identity.Authenticate(ctx, "ops@example.com", "secret")

	if err := svc.Suspend(ctx, tenantID, actor.ID, "request-1"); err != nil {
		t.Fatal(err)
	}

	var status string
	_ = s.Store.DB().QueryRow(`SELECT status FROM tenants WHERE id=?`, tenantID).Scan(&status)
	if status != "suspended" {
		t.Fatalf("tenant status=%s", status)
	}
	var active int
	_ = s.Store.DB().QueryRow(`SELECT active FROM users WHERE id=?`, target.ID).Scan(&active)
	if active != 0 {
		t.Fatalf("user active=%d", active)
	}
	if _, _, err := s.Identity.AuthenticateToken(ctx, session.TokenHash); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("session remains valid: %v", err)
	}
	var audits int
	_ = s.Store.DB().QueryRow(`SELECT COUNT(*) FROM audit_events WHERE aggregate_id=? AND action='suspend'`, tenantID).Scan(&audits)
	if audits != 1 {
		t.Fatalf("audits=%d", audits)
	}
}

func TestSuspendUnreachableWhenAuditStoreUnavailableLeavesStateRecoverable(t *testing.T) {
	s, svc := tenantServices(t)
	ctx := context.Background()
	tenantID, _ := s.Identity.CreateTenant(ctx, "compute-tenant")
	actor, _ := s.Identity.CreateUser(ctx, tenantID, "admin@example.com", "Admin", "secret", domain.RoleTenantAdmin)
	target, _ := s.Identity.CreateUser(ctx, tenantID, "ops@example.com", "Ops", "secret", domain.RoleClusterOps)
	session, _, _ := s.Identity.Authenticate(ctx, "ops@example.com", "secret")

	// Simulate audit storage being temporarily unavailable by installing a trigger
	// that aborts any audit_events insert. Because Suspend composes the tenant state,
	// identity suspension, and the audit write in one transaction, the aborted audit
	// write must roll back the tenant and identity changes together.
	if _, err := s.Store.DB().Exec(`CREATE TRIGGER audit_unavailable BEFORE INSERT ON audit_events BEGIN SELECT RAISE(ABORT, 'audit store unavailable'); END`); err != nil {
		t.Fatal(err)
	}

	err := svc.Suspend(ctx, tenantID, actor.ID, "request-1")
	if err == nil {
		t.Fatal("expected suspend to fail when audit store is unavailable")
	}
	if !strings.Contains(err.Error(), "audit store unavailable") {
		t.Fatalf("expected audit unavailable error, got %v", err)
	}

	var status string
	_ = s.Store.DB().QueryRow(`SELECT status FROM tenants WHERE id=?`, tenantID).Scan(&status)
	if status != "active" {
		t.Fatalf("tenant must remain recoverable, status=%s", status)
	}
	var active int
	_ = s.Store.DB().QueryRow(`SELECT active FROM users WHERE id=?`, target.ID).Scan(&active)
	if active != 1 {
		t.Fatalf("identity must remain recoverable, user active=%d", active)
	}
	if _, _, err := s.Identity.AuthenticateToken(ctx, session.TokenHash); err != nil {
		t.Fatalf("session must remain valid when suspend rolled back: %v", err)
	}
	var audits int
	_ = s.Store.DB().QueryRow(`SELECT COUNT(*) FROM audit_events WHERE aggregate_id=? AND action='suspend'`, tenantID).Scan(&audits)
	if audits != 0 {
		t.Fatalf("no partial audit should remain, audits=%d", audits)
	}

	// Drop the trigger and confirm the workflow now completes end-to-end, proving
	// both the tenant and identities were left in a state that fully recovers.
	if _, err := s.Store.DB().Exec(`DROP TRIGGER audit_unavailable`); err != nil {
		t.Fatal(err)
	}
	if err := svc.Suspend(ctx, tenantID, actor.ID, "request-2"); err != nil {
		t.Fatal(err)
	}
	_ = s.Store.DB().QueryRow(`SELECT status FROM tenants WHERE id=?`, tenantID).Scan(&status)
	if status != "suspended" {
		t.Fatalf("tenant status=%s after recovery", status)
	}
}

func TestSuspendRejectsUnknownAndAlreadySuspended(t *testing.T) {
	s, svc := tenantServices(t)
	ctx := context.Background()
	tenantID, _ := s.Identity.CreateTenant(ctx, "compute-tenant")
	actor, _ := s.Identity.CreateUser(ctx, tenantID, "admin@example.com", "Admin", "secret", domain.RoleTenantAdmin)

	if err := svc.Suspend(ctx, "missing", actor.ID, "r"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown tenant err=%v", err)
	}
	if err := svc.Suspend(ctx, tenantID, actor.ID, "r"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Suspend(ctx, tenantID, actor.ID, "r2"); !errors.Is(err, domain.ErrState) {
		t.Fatalf("already suspended err=%v", err)
	}
}
