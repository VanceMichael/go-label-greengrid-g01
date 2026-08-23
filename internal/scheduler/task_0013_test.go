package scheduler_test

import (
	"context"
	"testing"
	"time"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/scheduler"
	"github.com/VanceMichael/greengrid/internal/testsupport"
)

func TestGreenGridTask0013(t *testing.T) {
	s, err := testsupport.Open(t.TempDir() + "/task.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Store.Close()
	ctx := context.Background()
	tenantID, _ := s.Identity.CreateTenant(ctx, "batch-cancel")
	u, _ := s.Identity.CreateUser(ctx, tenantID, "scheduler@example.com", "Scheduler", "secret", domain.RoleScheduler)
	c, _ := s.Cluster.CreateCluster(ctx, tenantID, "cluster", "region", 8)
	now := time.Now().UTC().Truncate(time.Second)
	r, _ := s.Reservation.Request(ctx, tenantID, u.ID, c.ID, 2, now.Add(time.Hour), now.Add(2*time.Hour), "request")
	if err := s.Reservation.Approve(ctx, tenantID, u.ID, r.ID, 1, "approve"); err != nil {
		t.Fatal(err)
	}
	if err := s.Reservation.Activate(ctx, tenantID, r.ID, 2, "activate"); err != nil {
		t.Fatal(err)
	}
	j, err := s.Job.Submit(ctx, tenantID, r.ID, "", "batch-job", 1, u.ID, "submit")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Job.Claim(ctx, "worker-13", now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Store.DB().Exec(`CREATE TRIGGER fail_batch_audit BEFORE INSERT ON audit_events WHEN NEW.action='batch_cancel' BEGIN SELECT RAISE(ABORT,'audit unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	results := scheduler.NewService(s.Store).CancelBatch(ctx, tenantID, u.ID, "cancel", []string{j.ID})
	if len(results) != 1 || results[0].Err == nil {
		t.Fatalf("results=%+v", results)
	}
	var status string
	if err := s.Store.DB().QueryRow(`SELECT status FROM jobs WHERE id=?`, j.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	var leases int
	if err := s.Store.DB().QueryRow(`SELECT COUNT(*) FROM leases WHERE resource_type='job' AND resource_id=?`, j.ID).Scan(&leases); err != nil {
		t.Fatal(err)
	}
	if status != string(domain.JobClaimed) || leases != 1 {
		t.Fatalf("partial cancellation status=%s leases=%d", status, leases)
	}
}
