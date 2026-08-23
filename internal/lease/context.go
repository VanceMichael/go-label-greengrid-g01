package lease

import "context"

// acquisitionContext returns the context under which a lease acquisition
// transaction runs. It must NOT detach from the caller's cancellation: a
// worker that is shutting down must never commit a lease it still owns.
func acquisitionContext(ctx context.Context) context.Context {
	return ctx
}
