package ports

import (
	"context"

	"outages-bot/internal/notifier/application/core/domain"
)

// ScheduleFeedPort is the driven port for the provider's published schedules.
type ScheduleFeedPort interface {
	FetchSchedules(context.Context) ([]domain.Schedule, error)
}

// ScheduleSenderPort delivers schedules to one chat. today lets the adapter
// label days relatively.
type ScheduleSenderPort interface {
	SendSchedules(ctx context.Context, chatID int64, today string, schedules []domain.Schedule) error
}
