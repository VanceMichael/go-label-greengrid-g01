package job_test

import (
	"context"
	"testing"
	"time"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/testsupport"
)

func TestGreenGridTask0012(t *testing.T) {
	s, err := testsupport.Open(t.TempDir() + "/task.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Store.Close()
	ctx := context.Background()
	tenantID, _ := s.Identity.CreateTenant(ctx, "attempt-atomicity")
	u, _ := s.Identity.CreateUser(ctx, tenantID, "scheduler@example.com", "Scheduler", "secret", domain.RoleScheduler)
	c, _ := s.Cluster.CreateCluster(ctx, tenantID, "cluster", "region", 8)
	now := time.Now().UTC().Truncate(time.Second)
	r, _ := s.Reservation.Request(ctx, tenantID, u.ID, c.ID, 2, now.Add(time.Hour), now.Add(2*time.Hour), "r")
	if err := s.Reservation.Approve(ctx, tenantID, u.ID, r.ID, 1, "approve"); err != nil {
		t.Fatal(err)
	}
	if err := s.Reservation.Activate(ctx, tenantID, r.ID, 2, "activate"); err != nil {
		t.Fatal(err)
	}
	j, err := s.Job.Submit(ctx, tenantID, r.ID, "", "atomic-finish", 1, u.ID, "submit")
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := s.Job.Claim(ctx, "worker-12", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Job.Start(ctx, "worker-12", j.ID, claimed.Version); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Store.DB().Exec(`CREATE TRIGGER fail_attempt_history BEFORE INSERT ON job_attempts BEGIN SELECT RAISE(ABORT,'attempt history unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.Job.Finish(ctx, "worker-12", j.ID, claimed.Version+1, true, "", "finish"); err == nil {
		t.Fatal("finish unexpectedly succeeded")
	}
	var status string
	if err := s.Store.DB().QueryRow(`SELECT status FROM jobs WHERE id=?`, j.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	var leases, attempts int
	if err := s.Store.DB().QueryRow(`SELECT COUNT(*) FROM leases WHERE resource_type='job' AND resource_id=?`, j.ID).Scan(&leases); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.DB().QueryRow(`SELECT COUNT(*) FROM job_attempts WHERE job_id=?`, j.ID).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if status != string(domain.JobRunning) || leases != 1 || attempts != 0 {
		t.Fatalf("partial finish status=%s leases=%d attempts=%d", status, leases, attempts)
	}
}
