package lease_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/VanceMichael/greengrid/internal/lease"
	"github.com/VanceMichael/greengrid/internal/testsupport"
)

func TestGreenGridTask0006(t *testing.T) {
	s, err := testsupport.Open(t.TempDir() + "/task.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Store.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = lease.NewService(s.Store).Acquire(ctx, "job", "job-1", "worker-a", time.Now().UTC(), time.Now().UTC().Add(time.Minute))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled lease acquisition error=%v", err)
	}
	var count int
	if err := s.Store.DB().QueryRow(`SELECT COUNT(*) FROM leases`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("lease persisted after cancellation: %d", count)
	}
}
