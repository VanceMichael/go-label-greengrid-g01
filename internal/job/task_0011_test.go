package job_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/job"
)

type failingExecutor struct{}

func (failingExecutor) Execute(context.Context, domain.Job) (job.ExecutionResult, error) {
	return job.ExecutionResult{}, errors.New("temporary executor failure")
}

func TestGreenGridTask0011(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := job.ExecuteWithPolicy(ctx, failingExecutor{}, domain.Job{ID: "job-retry"}, job.AttemptPolicy{MaxAttempts: 3, BaseDelay: time.Second, MaxDelay: time.Second})
		result <- err
	}()
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("retry cancellation error=%v", err)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("retry backoff ignored cancellation")
	}
}
