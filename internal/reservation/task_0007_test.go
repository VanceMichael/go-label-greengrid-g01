package reservation_test

import (
	"context"
	"testing"
	"time"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/pagination"
	"github.com/VanceMichael/greengrid/internal/reservation"
	"github.com/VanceMichael/greengrid/internal/testsupport"
)

func TestGreenGridTask0007(t *testing.T) {
	s, err := testsupport.Open(t.TempDir() + "/task.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Store.Close()
	ctx := context.Background()
	tenantID, _ := s.Identity.CreateTenant(ctx, "reservation-list")
	u, _ := s.Identity.CreateUser(ctx, tenantID, "scheduler@example.com", "Scheduler", "secret", domain.RoleScheduler)
	c, _ := s.Cluster.CreateCluster(ctx, tenantID, "cluster", "region", 8)
	start := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	first, err := s.Reservation.Request(ctx, tenantID, u.ID, c.ID, 1, start, start.Add(time.Hour), "one")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Reservation.Approve(ctx, tenantID, u.ID, first.ID, 1, "approve"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Reservation.Request(ctx, tenantID, u.ID, c.ID, 1, start.Add(2*time.Hour), start.Add(3*time.Hour), "two"); err != nil {
		t.Fatal(err)
	}
	page, err := reservation.NewQuery(s.Store).List(ctx, tenantID, string(domain.ReservationRequested), pagination.Page{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if page.Meta.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("page=%+v", page)
	}
}
