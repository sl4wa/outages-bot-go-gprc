package ports

import (
	"context"

	"outages-bot/internal/notifier/application/core/domain"
)

type SchedulePort interface {
	FetchSchedules(context.Context) ([]domain.Schedule, error)
}
