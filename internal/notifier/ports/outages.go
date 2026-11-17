package ports

import (
	"context"

	"outages-bot/internal/notifier/application/core/domain"
)

type OutagePort interface {
	FetchSnapshot(context.Context) ([]domain.Outage, error)
}
