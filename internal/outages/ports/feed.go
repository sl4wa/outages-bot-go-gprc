package ports

import (
	"context"

	"outages-bot/internal/outages/application/core/domain"
)

type FeedPort interface {
	Fetch(context.Context) ([]domain.Outage, error)
}
