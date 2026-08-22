package lease

import "context"

func acquisitionContext(ctx context.Context) context.Context {
	return context.WithoutCancel(ctx)
}
