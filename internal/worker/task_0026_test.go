package worker_test

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/testsupport"
	"github.com/VanceMichael/greengrid/internal/worker"
)

type blockingSender struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *blockingSender) Send(context.Context, domain.OutboxEvent) error {
	s.once.Do(func() { close(s.started) })
	<-s.release
	return nil
}

func TestGreenGridTask0026(t *testing.T) {
	s, err := testsupport.Open(t.TempDir() + "/task.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Store.Close()
	ctx := context.Background()
	tenantID, _ := s.Identity.CreateTenant(ctx, "worker-stop")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.Store.DB().Exec(`INSERT INTO outbox_events(id,tenant_id,kind,aggregate_id,payload,status,attempts,next_attempt_at,created_at) VALUES('event',?,?,?,?, 'pending',0,?,?)`, tenantID, "test", "aggregate", "{}", now, now); err != nil {
		t.Fatal(err)
	}
	sender := &blockingSender{started: make(chan struct{}), release: make(chan struct{})}
	runner := worker.New(s.Job, s.Outbox, sender, time.Millisecond, "worker", slog.Default())
	runCtx, cancel := context.WithCancel(ctx)
	runner.Start(runCtx)
	select {
	case <-sender.started:
	case <-time.After(time.Second):
		t.Fatal("sender did not start")
	}
	stopCtx, stopCancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer stopCancel()
	done := make(chan error, 1)
	go func() { done <- runner.Stop(stopCtx) }()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("stop error=%v", err)
		}
	case <-time.After(80 * time.Millisecond):
		t.Fatal("worker stop ignored deadline")
	}
	cancel()
	close(sender.release)
}
