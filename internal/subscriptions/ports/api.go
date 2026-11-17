package ports

import (
	"context"

	"outages-bot/internal/subscriptions/application/core/domain"
)

type APIPort interface {
	SearchStreets(context.Context, string) (domain.StreetSearch, error)
	GetSubscription(context.Context, int64) (domain.Subscription, error)
	SetSubscription(context.Context, int64, int32, string) (domain.Subscription, error)
	DeleteSubscription(context.Context, int64, *domain.SubscriptionGuard) (bool, error)
	ListSubscriptions(context.Context) ([]domain.Subscription, error)
	MarkNotified(context.Context, int64, domain.SubscriptionGuard, *domain.Notification) (bool, error)
}
