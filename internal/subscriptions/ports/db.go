package ports

import (
	"context"

	"outages-bot/internal/subscriptions/application/core/domain"
)

type DBPort interface {
	Get(context.Context, int64) (domain.Subscription, error)
	Save(context.Context, domain.Subscription) error
	Delete(context.Context, int64) error
	List(context.Context) ([]domain.Subscription, error)
}
