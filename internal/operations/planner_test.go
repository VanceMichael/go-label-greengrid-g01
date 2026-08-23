package operations_test

import (
	"context"
	"testing"
	"time"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/operations"
	"github.com/VanceMichael/greengrid/internal/testsupport"
)

func plannerServices(t *testing.T) testsupport.Services {
	s, err := testsupport.Open(t.TempDir() + "/planner.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Store.Close() })
	return s
}

func TestReserveNodesRollsBackNodeCapacityOnAuditFailure(t *testing.T) {
	s := plannerServices(t)
	ctx := context.Background()
	tenant, _ := s.Identity.CreateTenant(ctx, "tenant")
	ops, _ := s.Identity.CreateUser(ctx, tenant, "ops@example.com", "Ops", "secret", domain.RoleClusterOps)
	c, _ := s.Cluster.CreateCluster(ctx, tenant, "cluster", "region", 8)
	// Two nodes with 4 GPUs each: 8 free total, reservation needs 6 across both.
	s.Cluster.AddNode(ctx, ops.ID, tenant, c.ID, "n0", 4, "register-n0")
	s.Cluster.AddNode(ctx, ops.ID, tenant, c.ID, "n1", 4, "register-n1")

	planner := operations.NewPlanner(s.Store)
	start := time.Now().UTC().Add(time.Hour)
	end := start.Add(time.Hour)

	// Force the plan audit write to fail. This models "audit write failed"
	// after the multi-node reservation has already consumed capacity.
	if _, err := s.Store.DB().ExecContext(ctx, `DROP TABLE audit_events`); err != nil {
		t.Fatal(err)
	}

	_, err := planner.ReserveNodes(ctx, tenant, c.ID, 6, start, end, ops.ID, "reserve")
	if err == nil {
		t.Fatal("expected audit failure to abort reservation")
	}

	// All consumed node capacity must be returned.
	var reserved0, reserved1 int
	if err := s.Store.DB().QueryRowContext(ctx, `SELECT gpu_reserved FROM nodes WHERE name='n0'`).Scan(&reserved0); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.DB().QueryRowContext(ctx, `SELECT gpu_reserved FROM nodes WHERE name='n1'`).Scan(&reserved1); err != nil {
		t.Fatal(err)
	}
	if reserved0 != 0 || reserved1 != 0 {
		t.Fatalf("capacity leaked after audit failure: n0=%d n1=%d", reserved0, reserved1)
	}

	// The next plan must not see phantom reserved space.
	plan, err := planner.Preview(ctx, tenant, c.ID, 6, start, end)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Accepted {
		t.Fatalf("next plan rejected by phantom capacity: %+v", plan)
	}
}
