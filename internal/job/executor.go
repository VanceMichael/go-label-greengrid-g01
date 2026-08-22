package job

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/VanceMichael/greengrid/internal/domain"
)

type Executor interface {
	Execute(context.Context, domain.Job) (ExecutionResult, error)
}

type ExecutionResult struct {
	JobID      string
	WorkerID   string
	ExitCode   int
	OutputRef  string
	StartedAt  time.Time
	FinishedAt time.Time
	Retryable  bool
	Message    string
}

type AttemptPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
}

func DefaultAttemptPolicy() AttemptPolicy {
	return AttemptPolicy{MaxAttempts: 4, BaseDelay: time.Second, MaxDelay: time.Minute}
}

func (p AttemptPolicy) Validate() error {
	if p.MaxAttempts < 1 || p.MaxAttempts > 100 {
		return fmt.Errorf("%w: max attempts", domain.ErrInvalid)
	}
	if p.BaseDelay <= 0 || p.MaxDelay < p.BaseDelay {
		return fmt.Errorf("%w: retry delay", domain.ErrInvalid)
	}
	return nil
}

func (p AttemptPolicy) Delay(attempt int) time.Duration {
	if attempt < 1 {
		return p.BaseDelay
	}
	delay := p.BaseDelay
	for i := 1; i < attempt; i++ {
		if delay >= p.MaxDelay/2 {
			return p.MaxDelay
		}
		delay *= 2
	}
	if delay > p.MaxDelay {
		return p.MaxDelay
	}
	return delay
}

func (p AttemptPolicy) ShouldRetry(attempt int, err error) bool {
	if err == nil || attempt >= p.MaxAttempts {
		return false
	}
	return !strings.Contains(strings.ToLower(err.Error()), "cancel")
}

func ValidateExecutionResult(job domain.Job, result ExecutionResult) error {
	if result.JobID != job.ID {
		return fmt.Errorf("%w: executor returned another job", domain.ErrConflict)
	}
	if result.WorkerID == "" {
		return fmt.Errorf("%w: executor worker", domain.ErrInvalid)
	}
	if result.FinishedAt.Before(result.StartedAt) {
		return fmt.Errorf("%w: executor timestamps", domain.ErrInvalid)
	}
	if result.ExitCode != 0 && result.Message == "" {
		return fmt.Errorf("%w: failed execution needs message", domain.ErrInvalid)
	}
	return nil
}

type InMemoryExecutor struct {
	Started chan string
	Finish  chan ExecutionResult
}

func NewInMemoryExecutor() *InMemoryExecutor {
	return &InMemoryExecutor{
		Started: make(chan string, 16),
		Finish:  make(chan ExecutionResult, 16),
	}
}

func (e *InMemoryExecutor) Execute(ctx context.Context, job domain.Job) (ExecutionResult, error) {
	select {
	case e.Started <- job.ID:
	case <-ctx.Done():
		return ExecutionResult{}, ctx.Err()
	}
	select {
	case result := <-e.Finish:
		return result, ValidateExecutionResult(job, result)
	case <-ctx.Done():
		return ExecutionResult{}, ctx.Err()
	}
}

func ExecuteWithPolicy(ctx context.Context, executor Executor, job domain.Job, policy AttemptPolicy) (ExecutionResult, error) {
	if err := policy.Validate(); err != nil {
		return ExecutionResult{}, err
	}
	var last error
	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		result, err := executor.Execute(ctx, job)
		if err == nil {
			return result, nil
		}
		last = err
		if !policy.ShouldRetry(attempt, err) {
			break
		}
		timer := time.NewTimer(policy.Delay(attempt))
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ExecutionResult{}, ctx.Err()
		}
	}
	return ExecutionResult{}, fmt.Errorf("%w: %v", domain.ErrUnavailable, last)
}
