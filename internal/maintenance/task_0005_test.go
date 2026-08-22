package maintenance_test

import (
	"context"
	"testing"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/maintenance"
	"github.com/VanceMichael/greengrid/internal/testsupport"
)

func TestGreenGridTask0005(t *testing.T) {
	s, err := testsupport.Open(t.TempDir() + "/task.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Store.Close()
	ctx := context.Background()
	tenantID, _ := s.Identity.CreateTenant(ctx, "maintenance")
	ops, _ := s.Identity.CreateUser(ctx, tenantID, "ops@example.com", "Ops", "secret", domain.RoleClusterOps)
	c, _ := s.Cluster.CreateCluster(ctx, tenantID, "cluster", "region", 4)
	n, _ := s.Cluster.AddNode(ctx, ops.ID, tenantID, c.ID, "node-1", 4, "node")
	if _, err := s.Store.DB().Exec(`CREATE TRIGGER fail_maintenance_audit BEFORE INSERT ON audit_events WHEN NEW.action='maintenance' BEGIN SELECT RAISE(ABORT,'audit unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	if err := maintenance.NewService(s.Store).Begin(ctx, tenantID, ops.ID, n.ID, "request-0005"); err == nil {
		t.Fatal("maintenance unexpectedly succeeded")
	}
	got, err := s.Cluster.GetNode(ctx, tenantID, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "ready" {
		t.Fatalf("node status=%s after failed maintenance", got.Status)
	}
}
