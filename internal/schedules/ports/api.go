package ports

import (
	"context"

	"outages-bot/internal/schedules/application/core/domain"
)

type APIPort interface {
	FetchSchedules(context.Context) ([]domain.Schedule, error)
}
