package tenant_test

import (
	"context"
	"testing"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/tenant"
	"github.com/VanceMichael/greengrid/internal/testsupport"
)

func TestGreenGridTask0001(t *testing.T) {
	s, err := testsupport.Open(t.TempDir() + "/task.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Store.Close()
	ctx := context.Background()
	tenantID, _ := s.Identity.CreateTenant(ctx, "suspend-tenant")
	actor, _ := s.Identity.CreateUser(ctx, tenantID, "actor@example.com", "Actor", "secret", domain.RoleTenantAdmin)
	target, _ := s.Identity.CreateUser(ctx, tenantID, "target@example.com", "Target", "secret", domain.RoleScheduler)
	session, _, err := s.Identity.Authenticate(ctx, "target@example.com", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Store.DB().Exec(`CREATE TRIGGER fail_suspend_audit BEFORE INSERT ON audit_events WHEN NEW.action='suspend' BEGIN SELECT RAISE(ABORT,'audit unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	if err := tenant.NewService(s.Store).Suspend(ctx, tenantID, actor.ID, "request-0001"); err == nil {
		t.Fatal("suspend unexpectedly succeeded")
	}
	var status string
	if err := s.Store.DB().QueryRow(`SELECT status FROM tenants WHERE id=?`, tenantID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "active" {
		t.Fatalf("tenant status=%s after failed suspend", status)
	}
	var active int
	if err := s.Store.DB().QueryRow(`SELECT active FROM users WHERE id=?`, target.ID).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 1 {
		t.Fatalf("target active=%d after failed suspend", active)
	}
	var revoked int
	if err := s.Store.DB().QueryRow(`SELECT revoked FROM sessions WHERE id=?`, session.ID).Scan(&revoked); err != nil {
		t.Fatal(err)
	}
	if revoked != 0 {
		t.Fatalf("session revoked=%d after failed suspend", revoked)
	}
}
