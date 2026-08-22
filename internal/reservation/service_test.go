package reservation_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/testsupport"
)

func reservationServices(t *testing.T) testsupport.Services {
	s, err := testsupport.Open(t.TempDir() + "/reservation.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Store.Close() })
	return s
}
func reservationFixture(t *testing.T) (testsupport.Services, string, domain.User, domain.Cluster) {
	s := reservationServices(t)
	ctx := context.Background()
	tenant, _ := s.Identity.CreateTenant(ctx, "tenant")
	user, _ := s.Identity.CreateUser(ctx, tenant, "scheduler@example.com", "Scheduler", "secret", domain.RoleScheduler)
	cluster, _ := s.Cluster.CreateCluster(ctx, tenant, "green-cluster", "horinger", 8)
	return s, tenant, user, cluster
}
func approvedReservation(t *testing.T, s testsupport.Services, tenant string, u domain.User, c domain.Cluster) domain.Reservation {
	t.Helper()
	ctx := context.Background()
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
	r, err = s.Reservation.Get(ctx, tenant, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestReservationRequestApproveActivateAndRelease(t *testing.T) {
	s, tenant, u, c := reservationFixture(t)
	ctx := context.Background()
	r := approvedReservation(t, s, tenant, u, c)
	if r.Status != domain.ReservationActive || r.Version != 3 {
		t.Fatalf("reservation=%+v", r)
	}
	got, _ := s.Cluster.GetCluster(ctx, tenant, c.ID)
	if got.ReservedGPU != 4 {
		t.Fatalf("reserved=%d", got.ReservedGPU)
	}
	if err := s.Reservation.Release(ctx, tenant, u.ID, r.ID, r.Version, "release"); err != nil {
		t.Fatal(err)
	}
	released, _ := s.Reservation.Get(ctx, tenant, r.ID)
	if released.Status != domain.ReservationReleased {
		t.Fatalf("status=%s", released.Status)
	}
	cluster, _ := s.Cluster.GetCluster(ctx, tenant, c.ID)
	if cluster.ReservedGPU != 0 {
		t.Fatalf("capacity=%d", cluster.ReservedGPU)
	}
}

func TestReservationOverlapAndTimeBoundary(t *testing.T) {
	s, tenant, u, c := reservationFixture(t)
	ctx := context.Background()
	start := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	first, err := s.Reservation.Request(ctx, tenant, u.ID, c.ID, 2, start, start.Add(time.Hour), "one")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Reservation.Request(ctx, tenant, u.ID, c.ID, 2, start.Add(30*time.Minute), start.Add(90*time.Minute), "two"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("overlap err=%v", err)
	}
	second, err := s.Reservation.Request(ctx, tenant, u.ID, c.ID, 2, start.Add(time.Hour), start.Add(2*time.Hour), "boundary")
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == first.ID {
		t.Fatal("duplicate reservation")
	}
	rows, err := s.Reservation.ListOverlapping(ctx, tenant, c.ID, start.Add(15*time.Minute), start.Add(45*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != first.ID {
		t.Fatalf("overlap rows=%+v", rows)
	}
}

func TestReservationCancelRollsCapacityBack(t *testing.T) {
	s, tenant, u, c := reservationFixture(t)
	ctx := context.Background()
	r := approvedReservation(t, s, tenant, u, c)
	if err := s.Reservation.Cancel(ctx, tenant, u.ID, r.ID, r.Version, "cancel"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Cluster.GetCluster(ctx, tenant, c.ID)
	if got.ReservedGPU != 0 {
		t.Fatalf("capacity leaked=%d", got.ReservedGPU)
	}
	if err := s.Reservation.Cancel(ctx, tenant, u.ID, r.ID, r.Version, "again"); !errors.Is(err, domain.ErrState) && !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("second cancel err=%v", err)
	}
}

func TestReservationVersionConflict(t *testing.T) {
	s, tenant, u, c := reservationFixture(t)
	ctx := context.Background()
	r, err := s.Reservation.Request(ctx, tenant, u.ID, c.ID, 1, time.Now().UTC().Add(time.Hour), time.Now().UTC().Add(2*time.Hour), "r")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Reservation.Approve(ctx, tenant, u.ID, r.ID, 2, "wrong"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("version err=%v", err)
	}
	r2, _ := s.Reservation.Get(ctx, tenant, r.ID)
	if r2.Status != domain.ReservationRequested || r2.Version != 1 {
		t.Fatalf("mutated=%+v", r2)
	}
}

func TestConcurrentReservationRequestsHaveOneWinner(t *testing.T) {
	s, tenant, u, c := reservationFixture(t)
	start := time.Now().UTC().Add(3 * time.Hour).Truncate(time.Second)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Reservation.Request(context.Background(), tenant, u.ID, c.ID, 2, start, start.Add(time.Hour), "same-window")
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		if err == nil {
			success++
		}
		if errors.Is(err, domain.ErrConflict) {
			conflict++
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("success=%d conflict=%d", success, conflict)
	}
}
