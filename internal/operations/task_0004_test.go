package operations_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/operations"
	"github.com/VanceMichael/greengrid/internal/testsupport"
)

func TestGreenGridTask0004(t *testing.T) {
	s, err := testsupport.Open(t.TempDir() + "/task.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Store.Close()
	ctx := context.Background()
	tenantID, _ := s.Identity.CreateTenant(ctx, "planner-cache")
	ops, _ := s.Identity.CreateUser(ctx, tenantID, "ops@example.com", "Ops", "secret", domain.RoleClusterOps)
	c, _ := s.Cluster.CreateCluster(ctx, tenantID, "cluster", "region", 4)
	_, _ = s.Cluster.AddNode(ctx, ops.ID, tenantID, c.ID, "node-1", 4, "node")
	start := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	planner := operations.NewPlanner(s.Store)
	first, err := planner.Preview(ctx, tenantID, c.ID, 4, start, start.Add(time.Hour))
	if err != nil || !first.Accepted {
		t.Fatalf("first preview=%+v err=%v", first, err)
	}
	if err := s.Store.WithTx(ctx, func(tx *sql.Tx) error { return s.Cluster.ReserveCapacity(tx, c.ID, 4) }); err != nil {
		t.Fatal(err)
	}
	second, err := planner.Preview(ctx, tenantID, c.ID, 4, start, start.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if second.Accepted {
		t.Fatalf("stale preview accepted after capacity was consumed: %+v", second)
	}
}
