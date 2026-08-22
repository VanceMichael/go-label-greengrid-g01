package carbon_test

import (
	"context"
	"testing"
	"time"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/testsupport"
)

func TestGreenGridTask0018(t *testing.T) {
	s, err := testsupport.Open(t.TempDir() + "/task.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Store.Close()
	ctx := context.Background()
	tenantID, _ := s.Identity.CreateTenant(ctx, "carbon-approval")
	ops, _ := s.Identity.CreateUser(ctx, tenantID, "ops@example.com", "Ops", "secret", domain.RoleClusterOps)
	c, _ := s.Cluster.CreateCluster(ctx, tenantID, "cluster", "region", 8)
	n, _ := s.Cluster.AddNode(ctx, ops.ID, tenantID, c.ID, "node", 8, "node")
	start := time.Now().UTC().Truncate(time.Second)
	_, _ = s.Telemetry.Record(ctx, tenantID, n.ID, 1, start, 1000, .8, "reading")
	report, err := s.Carbon.Generate(ctx, tenantID, c.ID, start, start.Add(time.Hour), ops.ID, "generate")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Store.DB().Exec(`CREATE TRIGGER fail_carbon_outbox BEFORE INSERT ON outbox_events WHEN NEW.kind='carbon_report.approved' BEGIN SELECT RAISE(ABORT,'outbox unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.Carbon.Approve(ctx, tenantID, ops.ID, report.ID, 1, "approve"); err == nil {
		t.Fatal("approval unexpectedly succeeded")
	}
	var status string
	if err := s.Store.DB().QueryRow(`SELECT status FROM carbon_reports WHERE id=?`, report.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "draft" {
		t.Fatalf("report status=%s after outbox failure", status)
	}
}
