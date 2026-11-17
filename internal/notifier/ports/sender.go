package ports

import (
	"context"

	"outages-bot/internal/notifier/application/core/domain"
)

type SenderPort interface {
	Send(context.Context, int64, domain.Outage) error
}
