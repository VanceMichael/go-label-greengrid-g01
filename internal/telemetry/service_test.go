package telemetry_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/testsupport"
)

func telemetryServices(t *testing.T) testsupport.Services {
	s, err := testsupport.Open(t.TempDir() + "/telemetry.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Store.Close() })
	return s
}
func telemetryFixture(t *testing.T) (testsupport.Services, string, domain.User, domain.Node) {
	s := telemetryServices(t)
	ctx := context.Background()
	tenant, _ := s.Identity.CreateTenant(ctx, "tenant")
	ops, _ := s.Identity.CreateUser(ctx, tenant, "ops@example.com", "Ops", "secret", domain.RoleClusterOps)
	c, _ := s.Cluster.CreateCluster(ctx, tenant, "cluster", "region", 8)
	n, _ := s.Cluster.AddNode(ctx, ops.ID, tenant, c.ID, "node", 8, "request")
	return s, tenant, ops, n
}

func TestRecordReadingAndQueryWindow(t *testing.T) {
	s, tenant, _, n := telemetryFixture(t)
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Second)
	if _, err := s.Telemetry.Record(ctx, tenant, n.ID, 1, base, 1000, .75, "r1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Telemetry.Record(ctx, tenant, n.ID, 2, base.Add(time.Minute), 800, .5, "r2"); err != nil {
		t.Fatal(err)
	}
	rows, err := s.Telemetry.Readings(ctx, tenant, n.ID, base, base.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Sequence != 1 || rows[1].RenewableShare != .5 {
		t.Fatalf("rows=%+v", rows)
	}
}

func TestDuplicateSequenceIsIdempotentConflict(t *testing.T) {
	s, tenant, _, n := telemetryFixture(t)
	ctx := context.Background()
	now := time.Now().UTC()
	if _, err := s.Telemetry.Record(ctx, tenant, n.ID, 9, now, 100, .4, "r"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Telemetry.Record(ctx, tenant, n.ID, 9, now, 100, .4, "retry"); !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("duplicate err=%v", err)
	}
	var count int
	_ = s.Store.DB().QueryRow(`SELECT COUNT(*) FROM telemetry_readings WHERE node_id=?`, n.ID).Scan(&count)
	if count != 1 {
		t.Fatalf("count=%d", count)
	}
}

func TestTelemetryValidationAndTenantBoundary(t *testing.T) {
	s, tenant, _, n := telemetryFixture(t)
	ctx := context.Background()
	if _, err := s.Telemetry.Record(ctx, tenant, n.ID, 1, time.Now(), -1, .5, "r"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("negative power err=%v", err)
	}
	if _, err := s.Telemetry.Record(ctx, tenant, n.ID, 1, time.Now(), 1, 1.1, "r"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("share err=%v", err)
	}
	other, _ := s.Identity.CreateTenant(ctx, "other")
	if _, err := s.Telemetry.Record(ctx, other, n.ID, 1, time.Now(), 1, .5, "r"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("cross tenant err=%v", err)
	}
}
