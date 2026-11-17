package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"outages-bot/internal/bot/ports"
	"outages-bot/internal/platform/telegram"
)

const backoff = 5 * time.Second

type Transport interface {
	GetUpdates(context.Context, int64) ([]telegram.Update, error)
	SendReply(context.Context, int64, string, [][]string, bool) error
}

// Adapter polls Telegram and drives the application one message at a time.
// Polling progress is in memory and resets when the process restarts.
type Adapter struct {
	api       ports.APIPort
	transport Transport
	offset    int64
}

func NewAdapter(api ports.APIPort, transport Transport) *Adapter {
	return &Adapter{api: api, transport: transport}
}

func (a *Adapter) Run(ctx context.Context) error {
	for ctx.Err() == nil {
		updates, err := a.transport.GetUpdates(ctx, a.offset)
		if err == nil {
			err = a.processUpdates(ctx, updates)
		}
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			slog.Error("bot update processing failed", "error", err)
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil
			case <-timer.C:
			}
		}
	}
	return nil
}

func (a *Adapter) processUpdates(ctx context.Context, updates []telegram.Update) error {
	a.api.ExpireConversations()
	var sendErr error
	for _, update := range updates {
		if err := ctx.Err(); err != nil {
			return err
		}
		if update.UpdateID < a.offset {
			continue
		}
		if update.UpdateID == math.MaxInt64 {
			return errors.New("Telegram update ID cannot be incremented")
		}
		if msg := update.Message; msg != nil && msg.Chat.ID != 0 && msg.Text != "" {
			response, err := a.api.HandleText(ctx, msg.Chat.ID, msg.Text)
			if err != nil {
				return fmt.Errorf("handle update %d: %w", update.UpdateID, err)
			}
			if response != nil {
				if err := a.transport.SendReply(ctx, msg.Chat.ID, response.Text, response.Keyboard, response.RemoveKeyboard); err != nil {
					sendErr = fmt.Errorf("reply to update %d: %w", update.UpdateID, err)
				}
			}
		}
		// Conversation state may already have advanced. A failed reply must not
		// cause the same street input to be interpreted as a building on retry.
		a.offset = update.UpdateID + 1
	}
	return sendErr
}
