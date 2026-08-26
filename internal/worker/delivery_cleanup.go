package worker

import (
	"context"

	"github.com/VanceMichael/greengrid/internal/outbox"
)

func finishDelivery(ctx context.Context, events *outbox.Service, workerID, eventID string, sendErr error) error {
	return events.Finish(ctx, workerID, eventID, sendErr)
}
