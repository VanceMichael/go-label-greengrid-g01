package identity

import "context"

func authenticationQueryContext(ctx context.Context) context.Context {
	return context.WithoutCancel(ctx)
}
