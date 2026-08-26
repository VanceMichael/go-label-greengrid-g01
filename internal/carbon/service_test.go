package carbon_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/testsupport"
)

func carbonServices(t *testing.T) testsupport.Services {
	s, err := testsupport.Open(t.TempDir() + "/carbon.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Store.Close() })
	return s
}
func carbonFixture(t *testing.T) (testsupport.Services, string, domain.User, domain.Cluster, domain.Node) {
	s := carbonServices(t)
	ctx := context.Background()
	tenant, _ := s.Identity.CreateTenant(ctx, "tenant")
	ops, _ := s.Identity.CreateUser(ctx, tenant, "ops@example.com", "Ops", "secret", domain.RoleClusterOps)
	c, _ := s.Cluster.CreateCluster(ctx, tenant, "cluster", "region", 8)
	n, _ := s.Cluster.AddNode(ctx, ops.ID, tenant, c.ID, "node", 8, "r")
	return s, tenant, ops, c, n
}

func TestGenerateCarbonReportFromReadings(t *testing.T) {
	s, tenant, ops, c, n := carbonFixture(t)
	ctx := context.Background()
	start := time.Now().UTC().Truncate(time.Second)
	for i := int64(0); i < 3; i++ {
		if _, err := s.Telemetry.Record(ctx, tenant, n.ID, i, start.Add(time.Duration(i)*time.Minute), 1000, .8, "telemetry"); err != nil {
			t.Fatal(err)
		}
	}
	report, err := s.Carbon.Generate(ctx, tenant, c.ID, start, start.Add(5*time.Minute), ops.ID, "report")
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "draft" || report.EnergyKWh <= 0 || report.RenewableShare < .79 {
		t.Fatalf("report=%+v", report)
	}
	if err := s.Carbon.Approve(ctx, tenant, ops.ID, report.ID, 1, "approve"); err != nil {
		t.Fatal(err)
	}
	var status string
	_ = s.Store.DB().QueryRow(`SELECT status FROM carbon_reports WHERE id=?`, report.ID).Scan(&status)
	if status != "approved" {
		t.Fatalf("status=%s", status)
	}
}

func TestCarbonNeedsTelemetryAndTenantOwnership(t *testing.T) {
	s, tenant, ops, c, n := carbonFixture(t)
	ctx := context.Background()
	start := time.Now().UTC()
	if _, err := s.Carbon.Generate(ctx, tenant, c.ID, start, start.Add(time.Hour), ops.ID, "r"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("empty report err=%v", err)
	}
	other, _ := s.Identity.CreateTenant(ctx, "other")
	if _, err := s.Carbon.Generate(ctx, other, c.ID, start, start.Add(time.Hour), ops.ID, "r"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("cross tenant err=%v", err)
	}
	_ = n
}

func TestCarbonVersionConflictKeepsDraft(t *testing.T) {
	s, tenant, ops, c, n := carbonFixture(t)
	ctx := context.Background()
	start := time.Now().UTC()
	_, _ = s.Telemetry.Record(ctx, tenant, n.ID, 1, start, 500, .9, "r")
	report, _ := s.Carbon.Generate(ctx, tenant, c.ID, start, start.Add(time.Hour), ops.ID, "r")
	if err := s.Carbon.Approve(ctx, tenant, ops.ID, report.ID, 2, "wrong"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("conflict=%v", err)
	}
	var status string
	_ = s.Store.DB().QueryRow(`SELECT status FROM carbon_reports WHERE id=?`, report.ID).Scan(&status)
	if status != "draft" {
		t.Fatalf("status=%s", status)
	}
}
