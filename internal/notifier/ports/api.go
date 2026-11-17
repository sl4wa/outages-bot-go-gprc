package ports

import (
	"context"

	"outages-bot/internal/notifier/application/core/domain"
)

type APIPort interface {
	RunOnce(context.Context) (*domain.RunResult, error)
}
