package ports

import (
	"context"

	"outages-bot/internal/schedules/application/core/domain"
)

type FeedPort interface {
	Fetch(context.Context) ([]domain.Schedule, error)
}
