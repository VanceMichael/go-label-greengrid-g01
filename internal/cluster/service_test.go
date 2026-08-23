package cluster_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/testsupport"
)

func clusterServices(t *testing.T) testsupport.Services {
	s, err := testsupport.Open(t.TempDir() + "/cluster.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Store.Close() })
	return s
}

func TestClusterAndNodeLifecycle(t *testing.T) {
	s := clusterServices(t)
	ctx := context.Background()
	tenant, _ := s.Identity.CreateTenant(ctx, "green-tenant")
	ops, _ := s.Identity.CreateUser(ctx, tenant, "ops@example.com", "Ops", "secret", domain.RoleClusterOps)
	c, err := s.Cluster.CreateCluster(ctx, tenant, "horqin-a", "inner-mongolia", 16)
	if err != nil {
		t.Fatal(err)
	}
	n, err := s.Cluster.AddNode(ctx, ops.ID, tenant, c.ID, "gpu-01", 8, "request")
	if err != nil {
		t.Fatal(err)
	}
	if n.GPUCapacity != 8 || n.Status != "ready" {
		t.Fatalf("node=%+v", n)
	}
	got, err := s.Cluster.GetCluster(ctx, tenant, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CapacityGPU != 16 || got.ReservedGPU != 0 {
		t.Fatalf("cluster=%+v", got)
	}
	node, err := s.Cluster.GetNode(ctx, tenant, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if node.ClusterID != c.ID {
		t.Fatal(node)
	}
}

func TestClusterAndNodeTenantIsolation(t *testing.T) {
	s := clusterServices(t)
	ctx := context.Background()
	a, _ := s.Identity.CreateTenant(ctx, "a")
	b, _ := s.Identity.CreateTenant(ctx, "b")
	ops, _ := s.Identity.CreateUser(ctx, a, "ops@example.com", "Ops", "secret", domain.RoleClusterOps)
	c, _ := s.Cluster.CreateCluster(ctx, a, "cluster", "region", 4)
	if _, err := s.Cluster.GetCluster(ctx, b, c.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("cross tenant cluster err=%v", err)
	}
	if _, err := s.Cluster.AddNode(ctx, ops.ID, b, c.ID, "node", 2, "r"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("cross tenant node err=%v", err)
	}
}

func TestAddNodeAuditFailureLeavesNoNode(t *testing.T) {
	s := clusterServices(t)
	ctx := context.Background()
	tenant, _ := s.Identity.CreateTenant(ctx, "green-tenant")
	ops, _ := s.Identity.CreateUser(ctx, tenant, "ops@example.com", "Ops", "secret", domain.RoleClusterOps)
	c, _ := s.Cluster.CreateCluster(ctx, tenant, "horqin-a", "inner-mongolia", 16)

	// Force the audit write to fail by removing the audit_events table.
	if _, err := s.Store.DB().ExecContext(ctx, `DROP TABLE audit_events`); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Cluster.AddNode(ctx, ops.ID, tenant, c.ID, "gpu-01", 8, "request"); err == nil {
		t.Fatal("expected AddNode to fail when audit write fails")
	}

	// The node must not persist without an audit record.
	var nodeCount int
	if err := s.Store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM nodes`).Scan(&nodeCount); err != nil {
		t.Fatal(err)
	}
	if nodeCount != 0 {
		t.Fatalf("expected 0 nodes left after audit failure, got %d", nodeCount)
	}
}

func TestClusterCapacityConditions(t *testing.T) {
	s := clusterServices(t)
	ctx := context.Background()
	tenant, _ := s.Identity.CreateTenant(ctx, "a")
	c, _ := s.Cluster.CreateCluster(ctx, tenant, "cluster", "r", 4)
	if err := s.Store.WithTx(ctx, func(tx *sql.Tx) error { return s.Cluster.ReserveCapacity(tx, c.ID, 3) }); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.WithTx(ctx, func(tx *sql.Tx) error { return s.Cluster.ReserveCapacity(tx, c.ID, 2) }); !errors.Is(err, domain.ErrCapacity) {
		t.Fatalf("over capacity err=%v", err)
	}
	if err := s.Store.WithTx(ctx, func(tx *sql.Tx) error { return s.Cluster.ReleaseCapacity(tx, c.ID, 3) }); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Cluster.GetCluster(ctx, tenant, c.ID)
	if got.ReservedGPU != 0 {
		t.Fatalf("reserved=%d", got.ReservedGPU)
	}
}
