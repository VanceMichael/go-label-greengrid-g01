package app

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/VanceMichael/greengrid/internal/config"
	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/job"
	"github.com/VanceMichael/greengrid/internal/outbox"
	"github.com/VanceMichael/greengrid/internal/worker"
)

type shutdownSender struct {
	started chan struct{}
	release chan struct{}
}

func (s *shutdownSender) Send(context.Context, domain.OutboxEvent) error {
	close(s.started)
	<-s.release
	return nil
}

func TestGreenGridTask0025(t *testing.T) {
	cfg := config.Config{Address: "127.0.0.1:0", DatabasePath: t.TempDir() + "/shutdown.db", ShutdownTimeout: 200 * time.Millisecond, SessionTTL: time.Hour, WorkerInterval: time.Millisecond}
	r, err := New(cfg, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := r.store.DB().Exec(`INSERT INTO tenants(id,name,status,created_at) VALUES('tenant-0025','shutdown-tenant','active',?)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := r.store.DB().Exec(`INSERT INTO outbox_events(id,tenant_id,kind,aggregate_id,payload,status,attempts,next_attempt_at,created_at) VALUES('event-0025','tenant-0025','job.queued','job-0025','{}','pending',0,?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	sender := &shutdownSender{started: make(chan struct{}), release: make(chan struct{})}
	r.workers = worker.New(job.NewService(r.store), outbox.NewService(r.store, 3), sender, time.Millisecond, "worker-0025", slog.Default())
	parent, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- r.Run(parent) }()
	select {
	case <-sender.started:
	case <-time.After(time.Second):
		t.Fatal("worker did not reach the delivery step")
	}
	storeClosed := make(chan struct{})
	go func() {
		ticker := time.NewTicker(time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			if err := r.store.DB().Ping(); err != nil {
				close(storeClosed)
				return
			}
		}
	}()
	cancel()
	select {
	case <-storeClosed:
		t.Errorf("store closed while worker delivery was still blocked")
	case <-time.After(100 * time.Millisecond):
	}
	close(sender.release)
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("runtime shutdown error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("runtime did not finish shutdown")
	}
}
