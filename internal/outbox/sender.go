package outbox

import (
	"context"
	"log/slog"

	"github.com/VanceMichael/greengrid/internal/domain"
)

type LoggerSender struct{ logger *slog.Logger }

func NewLoggerSender(logger *slog.Logger) *LoggerSender { return &LoggerSender{logger: logger} }

func (s *LoggerSender) Send(ctx context.Context, event domain.OutboxEvent) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	s.logger.InfoContext(ctx, "deliver outbox", "event_id", event.ID, "kind", event.Kind, "aggregate_id", event.AggregateID, "payload", event.Payload)
	return nil
}
