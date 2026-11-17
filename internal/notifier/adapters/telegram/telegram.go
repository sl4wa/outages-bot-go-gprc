package telegram

import (
	"context"
	"errors"

	"outages-bot/internal/notifier/application/core/domain"
	"outages-bot/internal/notifier/ports"
	"outages-bot/internal/platform/telegram"
)

type Client interface {
	Send(context.Context, int64, string) error
}

type Adapter struct {
	client Client
}

var _ ports.SenderPort = (*Adapter)(nil)

func NewAdapter(client Client) *Adapter { return &Adapter{client: client} }

func (a *Adapter) Send(ctx context.Context, chatID int64, outage domain.Outage) error {
	err := a.client.Send(ctx, chatID, notificationText(outage))
	var telegramError *telegram.Error
	if errors.As(err, &telegramError) {
		switch telegramError.StatusCode {
		case 400:
			return domain.ErrRejected
		case 403:
			return domain.ErrBlocked
		case 429:
			return &domain.RateLimitError{RetryAfter: telegramError.RetryAfter}
		}
	}
	return err
}
