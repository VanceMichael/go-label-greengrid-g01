package operations_test

import (
	"context"
	"testing"
	"time"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/operations"
	"github.com/VanceMichael/greengrid/internal/testsupport"
)

func TestGreenGridTask0010(t *testing.T) {
	s, err := testsupport.Open(t.TempDir() + "/task.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Store.Close()
	ctx := context.Background()
	tenantID, _ := s.Identity.CreateTenant(ctx, "recovery")
	u, _ := s.Identity.CreateUser(ctx, tenantID, "scheduler@example.com", "Scheduler", "secret", domain.RoleScheduler)
	c, _ := s.Cluster.CreateCluster(ctx, tenantID, "cluster", "region", 8)
	now := time.Now().UTC().Truncate(time.Second)
	r, err := s.Reservation.Request(ctx, tenantID, u.ID, c.ID, 2, now.Add(time.Hour), now.Add(2*time.Hour), "reservation")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Store.DB().Exec(`UPDATE reservations SET status='active' WHERE id=?`, r.ID); err != nil {
		t.Fatal(err)
	}
	j, err := s.Job.Submit(ctx, tenantID, r.ID, "", "recover-me", 1, u.ID, "submit")
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := s.Job.Claim(ctx, "dead-worker", now.Add(-3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if claimed.ID != j.ID {
		t.Fatal(claimed)
	}
	actions, err := operations.NewRecoveryService(s.Store).RecoverJobs(ctx, tenantID, u.ID, "recover", now)
	if err != nil || len(actions) != 1 {
		t.Fatalf("actions=%+v err=%v", actions, err)
	}
	var status string
	if err := s.Store.DB().QueryRow(`SELECT status FROM jobs WHERE id=?`, j.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "queued" {
		t.Fatalf("status=%s", status)
	}
	var leases int
	if err := s.Store.DB().QueryRow(`SELECT COUNT(*) FROM leases WHERE resource_type='job' AND resource_id=?`, j.ID).Scan(&leases); err != nil {
		t.Fatal(err)
	}
	if leases != 0 {
		t.Fatalf("lease remains after recovery: %d", leases)
	}
}
