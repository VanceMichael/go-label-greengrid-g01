package cluster_test

import (
	"context"
	"testing"

	"github.com/VanceMichael/greengrid/internal/cluster"
	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/testsupport"
)

func TestGreenGridTask0003(t *testing.T) {
	s, err := testsupport.Open(t.TempDir() + "/task.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Store.Close()
	ctx := context.Background()
	tenantID, _ := s.Identity.CreateTenant(ctx, "audit-node")
	ops, _ := s.Identity.CreateUser(ctx, tenantID, "ops@example.com", "Ops", "secret", domain.RoleClusterOps)
	c, err := s.Cluster.CreateCluster(ctx, tenantID, "cluster", "region", 8)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Store.DB().Exec(`CREATE TRIGGER fail_register_audit BEFORE INSERT ON audit_events WHEN NEW.action='register' BEGIN SELECT RAISE(ABORT,'audit unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := cluster.NewService(s.Store).AddNode(ctx, ops.ID, tenantID, c.ID, "node-1", 4, "request-0003"); err == nil {
		t.Fatal("node registration unexpectedly succeeded")
	}
	var count int
	if err := s.Store.DB().QueryRow(`SELECT COUNT(*) FROM nodes WHERE cluster_id=?`, c.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("nodes persisted after audit failure: %d", count)
	}
}
