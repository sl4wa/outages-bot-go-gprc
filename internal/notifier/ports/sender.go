package ports

import (
	"context"

	"outages-bot/internal/notifier/application/core/domain"
)

type SenderPort interface {
	Send(context.Context, int64, domain.Outage) error
	// SendSchedules labels each schedule's day relative to today.
	SendSchedules(ctx context.Context, chatID int64, today string, schedules []domain.Schedule) error
}
