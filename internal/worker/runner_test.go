package worker_test

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/testsupport"
	"github.com/VanceMichael/greengrid/internal/worker"
)

type recordingSender struct {
	mu     sync.Mutex
	events []string
}

func (s *recordingSender) Send(ctx context.Context, e domain.OutboxEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, e.ID)
	return nil
}
func (s *recordingSender) Count() int { s.mu.Lock(); defer s.mu.Unlock(); return len(s.events) }

func TestRunnerStopsOnContextAndDrains(t *testing.T) {
	s, err := testsupport.Open(t.TempDir() + "/worker.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Store.Close()
	ctx := context.Background()
	tenant, _ := s.Identity.CreateTenant(ctx, "tenant")
	now := time.Now().UTC()
	_, err = s.Store.DB().Exec(`INSERT INTO outbox_events(id,tenant_id,kind,aggregate_id,payload,status,attempts,next_attempt_at,created_at) VALUES('event',?,?,?,?,?,0,?,?)`, tenant, "test", "aggregate", "{}", "pending", now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}
	sender := &recordingSender{}
	runner := worker.New(s.Job, s.Outbox, sender, 5*time.Millisecond, "worker", slog.Default())
	runCtx, cancel := context.WithCancel(ctx)
	runner.Start(runCtx)
	time.Sleep(40 * time.Millisecond)
	cancel()
	if err := runner.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sender.Count() != 1 {
		t.Fatalf("sent=%d", sender.Count())
	}
	count, _ := s.Outbox.Count(ctx, "sent")
	if count != 1 {
		t.Fatalf("outbox sent=%d", count)
	}
}
