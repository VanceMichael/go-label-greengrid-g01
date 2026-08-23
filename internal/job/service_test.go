package job_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/testsupport"
)

func jobServices(t *testing.T) testsupport.Services {
	s, err := testsupport.Open(t.TempDir() + "/job.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Store.Close() })
	return s
}
func activeJobFixture(t *testing.T) (testsupport.Services, string, domain.User, domain.Reservation) {
	s := jobServices(t)
	ctx := context.Background()
	tenant, _ := s.Identity.CreateTenant(ctx, "tenant")
	u, _ := s.Identity.CreateUser(ctx, tenant, "scheduler@example.com", "Scheduler", "secret", domain.RoleScheduler)
	c, _ := s.Cluster.CreateCluster(ctx, tenant, "cluster", "region", 8)
	now := time.Now().UTC().Truncate(time.Second)
	r, err := s.Reservation.Request(ctx, tenant, u.ID, c.ID, 4, now.Add(time.Hour), now.Add(2*time.Hour), "request")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Reservation.Approve(ctx, tenant, u.ID, r.ID, 1, "approve"); err != nil {
		t.Fatal(err)
	}
	if err := s.Reservation.Activate(ctx, tenant, r.ID, 2, "activate"); err != nil {
		t.Fatal(err)
	}
	r, _ = s.Reservation.Get(ctx, tenant, r.ID)
	return s, tenant, u, r
}

func TestSubmitRequiresActiveReservation(t *testing.T) {
	s, tenant, u, c := func() (testsupport.Services, string, domain.User, domain.Cluster) {
		s := jobServices(t)
		ctx := context.Background()
		tenant, _ := s.Identity.CreateTenant(ctx, "tenant")
		u, _ := s.Identity.CreateUser(ctx, tenant, "scheduler@example.com", "Scheduler", "secret", domain.RoleScheduler)
		c, _ := s.Cluster.CreateCluster(ctx, tenant, "cluster", "region", 8)
		return s, tenant, u, c
	}()
	_, err := s.Job.Submit(context.Background(), tenant, "missing", "", "job", 1, u.ID, "r")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing reservation=%v", err)
	}
	_ = c
}

func TestJobClaimStartFinishLifecycle(t *testing.T) {
	s, tenant, u, r := activeJobFixture(t)
	ctx := context.Background()
	j, err := s.Job.Submit(ctx, tenant, r.ID, "", "train-model", 2, u.ID, "submit")
	if err != nil {
		t.Fatal(err)
	}
	if j.Status != domain.JobQueued {
		t.Fatal(j)
	}
	count, err := s.Outbox.Count(ctx, "pending")
	if err != nil || count != 1 {
		t.Fatalf("outbox count=%d err=%v", count, err)
	}
	claimed, err := s.Job.Claim(ctx, "worker-a", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if claimed.ID != j.ID || claimed.Status != domain.JobClaimed {
		t.Fatalf("claimed=%+v", claimed)
	}
	if err := s.Job.Start(ctx, "worker-a", j.ID, claimed.Version); err != nil {
		t.Fatal(err)
	}
	if err := s.Job.Finish(ctx, "worker-a", j.ID, claimed.Version+1, true, "", "finish"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Job.Get(ctx, tenant, j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.JobSucceeded || got.FinishedAt == nil || got.Attempts != 1 {
		t.Fatalf("finished=%+v", got)
	}
	var leases int
	_ = s.Store.DB().QueryRow(`SELECT COUNT(*) FROM leases WHERE resource_id=?`, j.ID).Scan(&leases)
	if leases != 0 {
		t.Fatalf("lease remains=%d", leases)
	}
}

func TestJobClaimIsExclusiveAndWrongWorkerCannotFinish(t *testing.T) {
	s, tenant, u, r := activeJobFixture(t)
	ctx := context.Background()
	j, _ := s.Job.Submit(ctx, tenant, r.ID, "", "model", 1, u.ID, "submit")
	var wg sync.WaitGroup
	claimed := make(chan domain.Job, 2)
	errs := make(chan error, 2)
	for _, worker := range []string{"a", "b"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			v, err := s.Job.Claim(ctx, id, time.Now().UTC())
			claimed <- v
			errs <- err
		}(worker)
	}
	wg.Wait()
	close(claimed)
	close(errs)
	success := 0
	for err := range errs {
		if err == nil {
			success++
		} else if !errors.Is(err, domain.ErrNotFound) && !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("unexpected claim err=%v", err)
		}
	}
	if success != 1 {
		t.Fatalf("claim success=%d", success)
	}
	if err := s.Job.Finish(ctx, "not-owner", j.ID, 3, true, "", "finish"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("wrong owner err=%v", err)
	}
}

func TestJobCancelReleasesLeaseAndPreservesVersionOnConflict(t *testing.T) {
	s, tenant, u, r := activeJobFixture(t)
	ctx := context.Background()
	j, _ := s.Job.Submit(ctx, tenant, r.ID, "", "model", 1, u.ID, "submit")
	claimed, _ := s.Job.Claim(ctx, "worker", time.Now().UTC())
	if err := s.Job.Cancel(ctx, tenant, u.ID, j.ID, claimed.Version, "cancel"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Job.Get(ctx, tenant, j.ID)
	if got.Status != domain.JobCancelled {
		t.Fatalf("status=%s", got.Status)
	}
	if err := s.Job.Cancel(ctx, tenant, u.ID, j.ID, claimed.Version, "again"); !errors.Is(err, domain.ErrState) && !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("repeat cancel err=%v", err)
	}
}

func TestExpiredLeaseRequeuesRunningJob(t *testing.T) {
	s, tenant, u, r := activeJobFixture(t)
	ctx := context.Background()
	j, _ := s.Job.Submit(ctx, tenant, r.ID, "", "model", 1, u.ID, "submit")
	claimed, _ := s.Job.Claim(ctx, "worker", time.Now().UTC().Add(-3*time.Minute))
	if claimed.ID != j.ID {
		t.Fatal(claimed)
	}
	n, err := s.Job.RequeueExpired(ctx, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("requeued=%d", n)
	}
	got, _ := s.Job.Get(ctx, tenant, j.ID)
	if got.Status != domain.JobQueued {
		t.Fatalf("status=%s", got.Status)
	}
}

func TestJobStateRejectsFinishBeforeStart(t *testing.T) {
	s, tenant, u, r := activeJobFixture(t)
	ctx := context.Background()
	j, _ := s.Job.Submit(ctx, tenant, r.ID, "", "model", 1, u.ID, "submit")
	claimed, _ := s.Job.Claim(ctx, "worker", time.Now().UTC())
	if err := s.Job.Finish(ctx, "worker", j.ID, claimed.Version, true, "", "finish"); !errors.Is(err, domain.ErrState) {
		t.Fatalf("finish queued err=%v", err)
	}
}

func TestJobFinishAtomicWhenAttemptPersistFails(t *testing.T) {
	s, tenant, u, r := activeJobFixture(t)
	ctx := context.Background()
	j, _ := s.Job.Submit(ctx, tenant, r.ID, "", "model", 1, u.ID, "submit")
	claimed, _ := s.Job.Claim(ctx, "worker", time.Now().UTC())
	if err := s.Job.Start(ctx, "worker", j.ID, claimed.Version); err != nil {
		t.Fatal(err)
	}
	// Pre-seed a conflicting attempt row so persistAttempt's insert violates
	// the UNIQUE(job_id, attempt_no) constraint and fails. The terminal
	// transition and lease delete would otherwise succeed, so this isolates the
	// execution history write failure that must roll back the whole finish.
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.Store.DB().Exec(`INSERT INTO job_attempts(id,job_id,attempt_no,worker_id,status,started_at,finished_at) VALUES(?,?,?,?,?,?,?)`,
		"preseed", j.ID, claimed.Attempts, "worker", "running", now, now); err != nil {
		t.Fatal(err)
	}
	if err := s.Job.Finish(ctx, "worker", j.ID, claimed.Version+1, true, "", "finish"); err == nil {
		t.Fatal("finish succeeded despite attempt persist failure")
	}
	got, _ := s.Job.Get(ctx, tenant, j.ID)
	if got.Status != domain.JobRunning {
		t.Fatalf("job leaked to terminal status=%s while execution history write failed", got.Status)
	}
	var leases int
	_ = s.Store.DB().QueryRow(`SELECT COUNT(*) FROM leases WHERE resource_id=?`, j.ID).Scan(&leases)
	if leases != 1 {
		t.Fatalf("lease released despite failed finish=%d", leases)
	}
	var finishAudits int
	_ = s.Store.DB().QueryRow(`SELECT COUNT(*) FROM audit_events WHERE aggregate_id=? AND action='finish'`, j.ID).Scan(&finishAudits)
	if finishAudits != 0 {
		t.Fatalf("finish audit committed despite rollback=%d", finishAudits)
	}
}
