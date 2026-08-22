package reservation_test

import (
	"context"
	"testing"
	"time"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/testsupport"
)

func TestGreenGridTask0017(t *testing.T) {
	s, err := testsupport.Open(t.TempDir() + "/task.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Store.Close()
	ctx := context.Background()
	tenantID, _ := s.Identity.CreateTenant(ctx, "capacity-release")
	u, _ := s.Identity.CreateUser(ctx, tenantID, "operator@example.com", "Operator", "secret", domain.RoleScheduler)
	c, _ := s.Cluster.CreateCluster(ctx, tenantID, "cluster", "region", 8)
	now := time.Now().UTC().Truncate(time.Second)
	r, err := s.Reservation.Request(ctx, tenantID, u.ID, c.ID, 3, now.Add(time.Hour), now.Add(2*time.Hour), "request")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Reservation.Approve(ctx, tenantID, u.ID, r.ID, 1, "approve"); err != nil {
		t.Fatal(err)
	}
	if err := s.Reservation.Activate(ctx, tenantID, r.ID, 2, "activate"); err != nil {
		t.Fatal(err)
	}
	if err := s.Reservation.Cancel(ctx, tenantID, u.ID, r.ID, 3, "cancel"); err != nil {
		t.Fatal(err)
	}
	var status string
	var reserved int
	if err := s.Store.DB().QueryRow(`SELECT status FROM reservations WHERE id=?`, r.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.DB().QueryRow(`SELECT reserved_gpu FROM clusters WHERE id=?`, c.ID).Scan(&reserved); err != nil {
		t.Fatal(err)
	}
	if status != string(domain.ReservationCancelled) || reserved != 0 {
		t.Fatalf("cancelled reservation left capacity status=%s reserved=%d", status, reserved)
	}
}
