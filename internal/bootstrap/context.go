package bootstrap

import "context"

func provisioningContext(ctx context.Context) context.Context {
	return context.WithoutCancel(ctx)
}
