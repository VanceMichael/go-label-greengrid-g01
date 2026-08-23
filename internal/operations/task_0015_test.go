package operations_test

import (
	"context"
	"testing"
	"time"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/operations"
	"github.com/VanceMichael/greengrid/internal/testsupport"
)

func TestGreenGridTask0015(t *testing.T) {
	s, err := testsupport.Open(t.TempDir() + "/task.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Store.Close()
	ctx := context.Background()
	tenantID, _ := s.Identity.CreateTenant(ctx, "reconcile")
	u, _ := s.Identity.CreateUser(ctx, tenantID, "scheduler@example.com", "Scheduler", "secret", domain.RoleScheduler)
	c, _ := s.Cluster.CreateCluster(ctx, tenantID, "cluster", "region", 8)
	now := time.Now().UTC()
	r, _ := s.Reservation.Request(ctx, tenantID, u.ID, c.ID, 2, now.Add(time.Hour), now.Add(2*time.Hour), "r")
	if err := s.Reservation.Approve(ctx, tenantID, u.ID, r.ID, 1, "approve"); err != nil {
		t.Fatal(err)
	}
	if err := s.Reservation.Activate(ctx, tenantID, r.ID, 2, "activate"); err != nil {
		t.Fatal(err)
	}
	j, _ := s.Job.Submit(ctx, tenantID, r.ID, "", "job", 1, u.ID, "submit")
	if _, err := s.Job.Claim(ctx, "live-worker", now); err != nil {
		t.Fatal(err)
	}
	removed, err := operations.NewReconciler(s.Store).RepairExpiredLeases(ctx, tenantID, now)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 0 {
		t.Fatalf("removed live leases=%d", removed)
	}
	var count int
	if err := s.Store.DB().QueryRow(`SELECT COUNT(*) FROM leases WHERE resource_id=?`, j.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("live lease count=%d", count)
	}
}
