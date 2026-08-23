package maintenance_test

import (
	"context"
	"testing"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/maintenance"
	"github.com/VanceMichael/greengrid/internal/testsupport"
)

func maintenanceServices(t *testing.T) (testsupport.Services, *maintenance.Service) {
	t.Helper()
	s, err := testsupport.Open(t.TempDir() + "/maintenance.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Store.Close() })
	return s, maintenance.NewService(s.Store)
}

func nodeFixture(t *testing.T, s testsupport.Services) (string, domain.User, domain.Node) {
	t.Helper()
	ctx := context.Background()
	tenant, _ := s.Identity.CreateTenant(ctx, "tenant")
	ops, _ := testsupport.CreateUser(ctx, s, tenant, "ops@example.com", domain.RoleClusterOps)
	c, _ := s.Cluster.CreateCluster(ctx, tenant, "cluster", "region", 8)
	n, _ := s.Cluster.AddNode(ctx, ops.ID, tenant, c.ID, "node", 8, "request")
	return tenant, ops, n
}

func nodeStatus(t *testing.T, s testsupport.Services, nodeID string) string {
	t.Helper()
	var status string
	if err := s.Store.DB().QueryRow(`SELECT status FROM nodes WHERE id=?`, nodeID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func auditCount(t *testing.T, s testsupport.Services, nodeID, action string) int {
	t.Helper()
	var count int
	if err := s.Store.DB().QueryRow(`SELECT COUNT(*) FROM audit_events WHERE aggregate_id=? AND action=?`, nodeID, action).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestBeginFlipsNodeToMaintenanceAndAudits(t *testing.T) {
	s, m := maintenanceServices(t)
	tenant, ops, n := nodeFixture(t, s)
	ctx := context.Background()

	if err := m.Begin(ctx, tenant, ops.ID, n.ID, "req-1"); err != nil {
		t.Fatalf("begin: %v", err)
	}
	if status := nodeStatus(t, s, n.ID); status != "maintenance" {
		t.Fatalf("status=%s", status)
	}
	if count := auditCount(t, s, n.ID, "maintenance"); count != 1 {
		t.Fatalf("audit count=%d", count)
	}
}

// TestBeginAuditFailureRollsBackNodeForRetry is the regression for the
// cross-resource operation: if the maintenance audit write fails, the node
// must NOT be left in maintenance. It stays ready with no leaked audit row so
// the operator can retry the whole Begin call.
func TestBeginAuditFailureRollsBackNodeForRetry(t *testing.T) {
	s, m := maintenanceServices(t)
	tenant, ops, n := nodeFixture(t, s)
	ctx := context.Background()

	// Force only the maintenance audit insert to fail, simulating an audit
	// store outage at the moment the operator enters maintenance.
	if _, err := s.Store.DB().Exec(`CREATE TRIGGER audit_maintenance_fail BEFORE INSERT ON audit_events WHEN NEW.action='maintenance' BEGIN SELECT RAISE(ABORT, 'audit unavailable'); END`); err != nil {
		t.Fatal(err)
	}

	// Begin must fail because the dependent audit write fails.
	if err := m.Begin(ctx, tenant, ops.ID, n.ID, "req-1"); err == nil {
		t.Fatal("begin succeeded despite audit failure")
	}

	// The node must still be ready and the maintenance audit must not be
	// recorded, leaving capacity and node state retryable.
	if status := nodeStatus(t, s, n.ID); status != "ready" {
		t.Fatalf("node not rolled back: status=%s", status)
	}
	if count := auditCount(t, s, n.ID, "maintenance"); count != 0 {
		t.Fatalf("audit leaked: count=%d", count)
	}

	// Audit recovers; the operator retries the same operation and it succeeds.
	if _, err := s.Store.DB().Exec(`DROP TRIGGER audit_maintenance_fail`); err != nil {
		t.Fatal(err)
	}
	if err := m.Begin(ctx, tenant, ops.ID, n.ID, "req-2"); err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	if status := nodeStatus(t, s, n.ID); status != "maintenance" {
		t.Fatalf("retry status=%s", status)
	}
	if count := auditCount(t, s, n.ID, "maintenance"); count != 1 {
		t.Fatalf("retry audit count=%d", count)
	}
}

func TestBeginRejectsNonReadyNode(t *testing.T) {
	s, m := maintenanceServices(t)
	tenant, ops, n := nodeFixture(t, s)
	ctx := context.Background()

	if err := m.Begin(ctx, tenant, ops.ID, n.ID, "req-1"); err != nil {
		t.Fatal(err)
	}
	// A second Begin on the already-maintenance node must not flip version or
	// write a second audit row: the transition is guarded by status='ready'.
	if err := m.Begin(ctx, tenant, ops.ID, n.ID, "req-2"); err == nil {
		t.Fatal("second begin succeeded")
	}
	if status := nodeStatus(t, s, n.ID); status != "maintenance" {
		t.Fatalf("status changed: %s", status)
	}
	if count := auditCount(t, s, n.ID, "maintenance"); count != 1 {
		t.Fatalf("audit count=%d", count)
	}
}
