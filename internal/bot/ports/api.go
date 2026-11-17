package ports

import (
	"context"

	"outages-bot/internal/bot/application/core/domain"
)

type APIPort interface {
	HandleText(context.Context, int64, string) (*domain.Reply, error)
	ExpireConversations()
}
