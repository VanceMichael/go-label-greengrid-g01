package reservation_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/reservation"
	"github.com/VanceMichael/greengrid/internal/testsupport"
)

func TestGreenGridTask0008(t *testing.T) {
	s, err := testsupport.Open(t.TempDir() + "/task.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Store.Close()
	ctx := context.Background()
	tenantID, _ := s.Identity.CreateTenant(ctx, "cancel-query")
	u, _ := s.Identity.CreateUser(ctx, tenantID, "scheduler@example.com", "Scheduler", "secret", domain.RoleScheduler)
	c, _ := s.Cluster.CreateCluster(ctx, tenantID, "cluster", "region", 8)
	start := time.Now().UTC().Add(-time.Minute)
	if _, err := s.Reservation.Request(ctx, tenantID, u.ID, c.ID, 1, start, start.Add(time.Hour), "request"); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := reservation.NewQuery(s.Store).ActiveAt(cancelled, tenantID, c.ID, time.Now().UTC()); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled query error=%v", err)
	}
}
