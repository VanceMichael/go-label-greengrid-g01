package job

import (
	"context"
	"time"
)

func waitRetry(ctx context.Context, delay time.Duration) error {
	time.Sleep(delay)
	return ctx.Err()
}
