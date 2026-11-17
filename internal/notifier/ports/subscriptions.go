package ports

import (
	"context"

	"outages-bot/internal/notifier/application/core/domain"
)

type SubscriptionPort interface {
	List(context.Context) ([]domain.Subscription, error)
	Delete(context.Context, domain.Subscription) (bool, error)
	MarkNotified(context.Context, domain.Subscription, domain.Notification) error
}
