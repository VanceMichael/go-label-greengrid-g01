package tenant_test

import (
	"context"
	"errors"
	"testing"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/tenant"
	"github.com/VanceMichael/greengrid/internal/testsupport"
)

func TestGreenGridTask0020(t *testing.T) {
	s, err := testsupport.Open(t.TempDir() + "/task.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Store.Close()
	ctx := context.Background()
	tenantID, _ := s.Identity.CreateTenant(ctx, "cancel-suspend")
	admin, _ := s.Identity.CreateUser(ctx, tenantID, "admin@example.com", "Admin", "secret", domain.RoleTenantAdmin)
	_, _ = s.Identity.CreateUser(ctx, tenantID, "user@example.com", "User", "secret", domain.RoleScheduler)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := tenant.NewService(s.Store).Suspend(cancelled, tenantID, admin.ID, "cancel"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled suspend error=%v", err)
	}
	var status string
	if err := s.Store.DB().QueryRow(`SELECT status FROM tenants WHERE id=?`, tenantID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "active" {
		t.Fatalf("tenant status=%s after cancelled suspend", status)
	}
}
