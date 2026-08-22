package retention_test

import (
	"context"
	"testing"
	"time"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/retention"
	"github.com/VanceMichael/greengrid/internal/testsupport"
)

func TestGreenGridTask0019(t *testing.T) {
	s, err := testsupport.Open(t.TempDir() + "/task.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Store.Close()
	ctx := context.Background()
	tenantID, _ := s.Identity.CreateTenant(ctx, "retention")
	u, _ := s.Identity.CreateUser(ctx, tenantID, "admin@example.com", "Admin", "secret", domain.RoleTenantAdmin)
	a, v, _ := s.Artifact.Create(ctx, tenantID, "model", "sha256-12345678", 10, u.ID, "upload")
	if err := s.Artifact.Scan(ctx, tenantID, v.ID, true, u.ID, "scan"); err != nil {
		t.Fatal(err)
	}
	cluster, _ := s.Cluster.CreateCluster(ctx, tenantID, "cluster", "region", 8)
	now := time.Now().UTC()
	r, _ := s.Reservation.Request(ctx, tenantID, u.ID, cluster.ID, 2, now.Add(time.Hour), now.Add(2*time.Hour), "r")
	if _, err := s.Store.DB().Exec(`UPDATE reservations SET status='active' WHERE id=?`, r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Job.Submit(ctx, tenantID, r.ID, v.ID, "using-model", 1, u.ID, "job"); err != nil {
		t.Fatal(err)
	}
	if err := retention.NewService(s.Store).RetireVersion(ctx, tenantID, u.ID, v.ID, "retire"); err == nil {
		t.Fatalf("retired version %s of artifact %s while a job referenced it", v.ID, a.ID)
	}
}
