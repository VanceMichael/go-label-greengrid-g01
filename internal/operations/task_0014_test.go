package operations_test

import (
	"context"
	"testing"
	"time"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/operations"
	"github.com/VanceMichael/greengrid/internal/testsupport"
)

func TestGreenGridTask0014(t *testing.T) {
	s, err := testsupport.Open(t.TempDir() + "/task.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Store.Close()
	ctx := context.Background()
	tenantID, _ := s.Identity.CreateTenant(ctx, "plan-atomic")
	actor, _ := s.Identity.CreateUser(ctx, tenantID, "ops@example.com", "Ops", "secret", domain.RoleClusterOps)
	c, _ := s.Cluster.CreateCluster(ctx, tenantID, "cluster", "region", 8)
	_, _ = s.Cluster.AddNode(ctx, actor.ID, tenantID, c.ID, "node", 8, "node")
	if _, err := s.Store.DB().Exec(`CREATE TRIGGER fail_plan_audit BEFORE INSERT ON audit_events WHEN NEW.action='node_reserve' BEGIN SELECT RAISE(ABORT,'audit unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC().Add(time.Hour)
	if _, err := operations.NewPlanner(s.Store).ReserveNodes(ctx, tenantID, c.ID, 4, start, start.Add(time.Hour), actor.ID, "plan"); err == nil {
		t.Fatal("plan unexpectedly succeeded")
	}
	var reserved int
	if err := s.Store.DB().QueryRow(`SELECT gpu_reserved FROM nodes WHERE cluster_id=?`, c.ID).Scan(&reserved); err != nil {
		t.Fatal(err)
	}
	if reserved != 0 {
		t.Fatalf("node capacity=%d after audit failure", reserved)
	}
}
