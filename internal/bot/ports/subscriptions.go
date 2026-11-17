package ports

import (
	"context"

	"outages-bot/internal/bot/application/core/domain"
)

type SubscriptionPort interface {
	Get(context.Context, int64) (domain.Subscription, error)
	Set(context.Context, int64, int32, string) (domain.Subscription, error)
	Delete(context.Context, int64) (bool, error)
	SearchStreets(context.Context, string) (domain.StreetSearch, error)
}
