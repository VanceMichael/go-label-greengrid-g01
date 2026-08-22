package tenant

import "context"

func suspendContext(ctx context.Context) context.Context {
	return context.WithoutCancel(ctx)
}
