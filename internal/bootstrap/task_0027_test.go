package bootstrap_test

import (
	"context"
	"errors"
	"testing"

	"github.com/VanceMichael/greengrid/internal/bootstrap"
	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/testsupport"
)

func TestGreenGridTask0027(t *testing.T) {
	s, err := testsupport.Open(t.TempDir() + "/task.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Store.Close()
	ctx := context.Background()
	tenantID, _ := s.Identity.CreateTenant(ctx, "bootstrap-cancel")
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := bootstrap.ProvisionSupervisor(cancelled, s.Store, tenantID, "supervisor@example.com", "secret"); !errors.Is(err, context.Canceled) {
		t.Fatalf("bootstrap error=%v", err)
	}
	var count int
	if err := s.Store.DB().QueryRow(`SELECT COUNT(*) FROM users WHERE role=?`, domain.RolePlatformAdmin).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("supervisor persisted after cancelled bootstrap: %d", count)
	}
}
