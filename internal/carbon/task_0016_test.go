package carbon_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/VanceMichael/greengrid/internal/carbon"
	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/testsupport"
)

func TestGreenGridTask0016(t *testing.T) {
	s, err := testsupport.Open(t.TempDir() + "/task.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Store.Close()
	ctx := context.Background()
	tenantID, _ := s.Identity.CreateTenant(ctx, "carbon-cancel")
	ops, _ := s.Identity.CreateUser(ctx, tenantID, "ops@example.com", "Ops", "secret", domain.RoleClusterOps)
	c, _ := s.Cluster.CreateCluster(ctx, tenantID, "cluster", "region", 8)
	n, _ := s.Cluster.AddNode(ctx, ops.ID, tenantID, c.ID, "node", 8, "node")
	start := time.Now().UTC().Truncate(time.Second)
	if _, err := s.Telemetry.Record(ctx, tenantID, n.ID, 1, start, 1000, .8, "reading"); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := carbon.NewService(s.Store).Generate(cancelled, tenantID, c.ID, start, start.Add(time.Hour), ops.ID, "report"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled report error=%v", err)
	}
	var count int
	if err := s.Store.DB().QueryRow(`SELECT COUNT(*) FROM carbon_reports`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("partial report persisted: %d", count)
	}
}
