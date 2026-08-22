package sqlite

import "context"

func migrationContext(ctx context.Context) context.Context {
	return context.WithoutCancel(ctx)
}
