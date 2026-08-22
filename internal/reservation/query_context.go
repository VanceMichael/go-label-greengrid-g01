package reservation

import "context"

func activeReservationContext(ctx context.Context) context.Context {
	return context.WithoutCancel(ctx)
}
