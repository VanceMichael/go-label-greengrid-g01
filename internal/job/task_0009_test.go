package job_test

import (
	"context"
	"testing"
	"time"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/job"
	"github.com/VanceMichael/greengrid/internal/pagination"
	"github.com/VanceMichael/greengrid/internal/testsupport"
)

func TestGreenGridTask0009(t *testing.T) {
	s, err := testsupport.Open(t.TempDir() + "/task.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Store.Close()
	ctx := context.Background()
	tenantID, _ := s.Identity.CreateTenant(ctx, "job-list")
	u, _ := s.Identity.CreateUser(ctx, tenantID, "scheduler@example.com", "Scheduler", "secret", domain.RoleScheduler)
	c1, _ := s.Cluster.CreateCluster(ctx, tenantID, "cluster-a", "region", 8)
	c2, _ := s.Cluster.CreateCluster(ctx, tenantID, "cluster-b", "region", 8)
	now := time.Now().UTC().Truncate(time.Second)
	r1, _ := s.Reservation.Request(ctx, tenantID, u.ID, c1.ID, 1, now.Add(time.Hour), now.Add(2*time.Hour), "r1")
	r2, _ := s.Reservation.Request(ctx, tenantID, u.ID, c2.ID, 1, now.Add(3*time.Hour), now.Add(4*time.Hour), "r2")
	if _, err := s.Store.DB().Exec(`UPDATE reservations SET status='active' WHERE id IN (?,?)`, r1.ID, r2.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Store.DB().Exec(`INSERT INTO jobs(id,tenant_id,reservation_id,name,gpu_count,status,attempts,version,created_at) VALUES('job-a',?,?,?,1,'queued',0,1,?),('job-b',?,?,?,1,'queued',0,1,?)`, tenantID, r1.ID, "train-a", now.Format(time.RFC3339Nano), tenantID, r2.ID, "train-b", now.Add(time.Minute).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	page, err := job.NewQuery(s.Store).List(ctx, tenantID, string(domain.JobQueued), pagination.Page{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if page.Meta.Total != 1 || len(page.Items) != 1 || page.Items[0].ReservationID != r1.ID {
		t.Fatalf("page=%+v", page)
	}
}
