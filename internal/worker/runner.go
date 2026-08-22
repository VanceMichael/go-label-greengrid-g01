package worker

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/job"
	"github.com/VanceMichael/greengrid/internal/outbox"
)

type Sender interface {
	Send(context.Context, domain.OutboxEvent) error
}

type Runner struct {
	jobs     *job.Service
	outbox   *outbox.Service
	sender   Sender
	interval time.Duration
	logger   *slog.Logger
	workerID string
	wg       sync.WaitGroup
}

func New(jobs *job.Service, events *outbox.Service, sender Sender, interval time.Duration, workerID string, logger *slog.Logger) *Runner {
	return &Runner{jobs: jobs, outbox: events, sender: sender, interval: interval, workerID: workerID, logger: logger}
}

func (r *Runner) Start(ctx context.Context) {
	r.wg.Add(2)
	go r.jobLoop(ctx)
	go r.outboxLoop(ctx)
}

func (r *Runner) Wait() { r.wg.Wait() }

func (r *Runner) jobLoop(ctx context.Context) {
	defer r.wg.Done()
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.runJob(ctx)
		}
	}
}

func (r *Runner) outboxLoop(ctx context.Context) {
	defer r.wg.Done()
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.runOutbox(ctx)
		}
	}
}

func (r *Runner) runJob(ctx context.Context) {
	j, err := r.jobs.Claim(ctx, r.workerID, time.Now().UTC())
	if errors.Is(err, domain.ErrNotFound) {
		return
	}
	if err != nil {
		r.logger.Warn("claim job", "error", err)
		return
	}
	if err := r.jobs.Start(ctx, r.workerID, j.ID, j.Version); err != nil {
		r.logger.Warn("start job", "job_id", j.ID, "error", err)
		return
	}
	if err := r.jobs.Finish(ctx, r.workerID, j.ID, j.Version+1, true, "", j.ID); err != nil {
		r.logger.Warn("finish job", "job_id", j.ID, "error", err)
	}
}

func (r *Runner) runOutbox(ctx context.Context) {
	event, err := r.outbox.Claim(ctx, r.workerID, time.Now().UTC())
	if errors.Is(err, domain.ErrNotFound) {
		return
	}
	if err != nil {
		r.logger.Warn("claim outbox", "error", err)
		return
	}
	sendErr := r.sender.Send(ctx, event)
	if err := r.outbox.Finish(ctx, r.workerID, event.ID, sendErr); err != nil {
		r.logger.Warn("finish outbox", "event_id", event.ID, "error", err)
	}
}

func (r *Runner) Stop(ctx context.Context) error {
	done := make(chan struct{})
	go func() { r.wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
