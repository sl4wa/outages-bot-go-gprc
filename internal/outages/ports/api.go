package ports

import (
	"context"

	"outages-bot/internal/outages/application/core/domain"
)

type APIPort interface {
	FetchSnapshot(context.Context) ([]domain.Outage, error)
}
