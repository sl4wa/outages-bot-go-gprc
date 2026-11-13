// Package bot is the Telegram adapter. Subscription data is accessed only via
// the generated gRPC client. Conversations and polling progress live in memory.
package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	subscriptionsv1 "outages-bot/gen/subscriptions/v1"
	"outages-bot/internal/platform/telegram"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	rpcTimeout = 5 * time.Second
	pendingTTL = 30 * time.Minute
	backoff    = 5 * time.Second
)

type Transport interface {
	GetUpdates(context.Context, int64) ([]telegram.Update, error)
	SendReply(context.Context, int64, string, [][]string, bool) error
}

type Bot struct {
	client    subscriptionsv1.SubscriptionServiceClient
	transport Transport
	offset    int64
	pending   map[int64]conversation
	now       func() time.Time
}

type conversationStep uint8

const (
	searchStreet conversationStep = iota
	enterBuilding
)

type conversation struct {
	step    conversationStep
	street  *subscriptionsv1.Street
	started time.Time
}

// New creates a single-goroutine polling adapter. Restarting starts fresh
// conversations; durable subscriber records belong to the subscriptions service.
func New(client subscriptionsv1.SubscriptionServiceClient, transport Transport) *Bot {
	return &Bot{client: client, transport: transport, pending: make(map[int64]conversation), now: time.Now}
}

// Run long-polls Telegram until cancellation. Backend failures leave the failed
// update unconsumed, so a temporary gRPC outage cannot silently lose a command.
func (b *Bot) Run(ctx context.Context) error {
	for ctx.Err() == nil {
		updates, err := b.transport.GetUpdates(ctx, b.offset)
		if err == nil {
			err = b.processUpdates(ctx, updates)
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

type reply struct {
	text           string
	keyboard       [][]string
	removeKeyboard bool
}

func (b *Bot) processUpdates(ctx context.Context, updates []telegram.Update) error {
	// Polling also reclaims abandoned conversations, even without new messages.
	for chatID, state := range b.pending {
		if b.now().Sub(state.started) > pendingTTL {
			delete(b.pending, chatID)
		}
	}
	var sendErr error
	for _, update := range updates {
		if err := ctx.Err(); err != nil {
			return err
		}
		if update.UpdateID < b.offset {
			continue
		}
		if update.UpdateID == math.MaxInt64 {
			return errors.New("Telegram update ID cannot be incremented")
		}
		if msg := update.Message; msg != nil && msg.Chat.ID != 0 && msg.Text != "" {
			response, err := b.handleText(ctx, msg.Chat.ID, msg.Text)
			if err != nil {
				return fmt.Errorf("handle update %d: %w", update.UpdateID, err)
			}
			if response != nil {
				if err := b.transport.SendReply(ctx, msg.Chat.ID, response.text, response.keyboard, response.removeKeyboard); err != nil {
					sendErr = fmt.Errorf("reply to update %d: %w", update.UpdateID, err)
				}
			}
		}
		// Conversation state may already have advanced. A failed reply must not
		// cause the same street input to be interpreted as a building on retry.
		b.offset = update.UpdateID + 1
	}
	return sendErr
}

func (b *Bot) handleText(ctx context.Context, chatID int64, text string) (*reply, error) {
	switch command(text) {
	case "start":
		callCtx, cancel := context.WithTimeout(ctx, rpcTimeout)
		current, err := b.client.GetSubscription(callCtx, &subscriptionsv1.GetSubscriptionRequest{ChatId: chatID})
		cancel()
		if err != nil && status.Code(err) != codes.NotFound {
			return nil, err
		}
		b.pending[chatID] = conversation{step: searchStreet, started: b.now()}
		if current != nil {
			return &reply{text: fmt.Sprintf("Ваша поточна підписка:\nВулиця: %s\nБудинок: %s\n\nБудь ласка, введіть нову назву вулиці для оновлення підписки:", current.GetAddress().GetStreet().GetName(), current.GetAddress().GetBuilding()), removeKeyboard: true}, nil
		}
		return &reply{text: "Будь ласка, введіть назву вулиці:", removeKeyboard: true}, nil
	case "subscription":
		callCtx, cancel := context.WithTimeout(ctx, rpcTimeout)
		defer cancel()
		current, err := b.client.GetSubscription(callCtx, &subscriptionsv1.GetSubscriptionRequest{ChatId: chatID})
		if status.Code(err) == codes.NotFound {
			return &reply{text: "Ви не маєте активної підписки."}, nil
		}
		if err != nil {
			return nil, err
		}
		return &reply{text: fmt.Sprintf("Ваша поточна підписка:\nВулиця: %s\nБудинок: %s", current.GetAddress().GetStreet().GetName(), current.GetAddress().GetBuilding())}, nil
	case "stop":
		callCtx, cancel := context.WithTimeout(ctx, rpcTimeout)
		result, err := b.client.DeleteSubscription(callCtx, &subscriptionsv1.DeleteSubscriptionRequest{ChatId: chatID})
		cancel()
		if err != nil {
			return nil, err
		}
		delete(b.pending, chatID)
		if result.GetDeleted() {
			return &reply{text: "Ви успішно відписалися від сповіщень про відключення електроенергії.", removeKeyboard: true}, nil
		}
		return &reply{text: "Ви не маєте активної підписки.", removeKeyboard: true}, nil
	}
	state, ok := b.pending[chatID]
	if !ok {
		return nil, nil
	}
	switch state.step {
	case searchStreet:
		return b.searchStreet(ctx, chatID, text, state)
	case enterBuilding:
		return b.saveSubscription(ctx, chatID, text, state)
	default:
		return nil, fmt.Errorf("unknown conversation step %d", state.step)
	}
}

func (b *Bot) searchStreet(ctx context.Context, chatID int64, text string, state conversation) (*reply, error) {
	query := strings.TrimSpace(text)
	if query == "" {
		return &reply{text: "Введіть назву вулиці."}, nil
	}
	callCtx, cancel := context.WithTimeout(ctx, rpcTimeout)
	result, err := b.client.SearchStreets(callCtx, &subscriptionsv1.SearchStreetsRequest{Query: query})
	cancel()
	if err != nil {
		return nil, err
	}
	if result.GetTooMany() {
		return &reply{text: "Знайдено забагато вулиць. Введіть точнішу назву:"}, nil
	}
	switch len(result.GetStreets()) {
	case 0:
		return &reply{text: "Вулицю не знайдено. Спробуйте ще раз."}, nil
	case 1:
		street := result.Streets[0]
		state.step, state.street = enterBuilding, street
		b.pending[chatID] = state
		return &reply{text: fmt.Sprintf("Ви обрали вулицю: %s\nБудь ласка, введіть номер будинку:", street.Name), removeKeyboard: true}, nil
	default:
		keyboard := make([][]string, len(result.Streets))
		for i, street := range result.Streets {
			keyboard[i] = []string{street.Name}
		}
		return &reply{text: "Будь ласка, оберіть вулицю:", keyboard: keyboard}, nil
	}
}

func (b *Bot) saveSubscription(ctx context.Context, chatID int64, text string, state conversation) (*reply, error) {
	if strings.TrimSpace(text) == "" {
		return &reply{text: "номер будинку не може бути порожнім"}, nil
	}
	callCtx, cancel := context.WithTimeout(ctx, rpcTimeout)
	result, err := b.client.SetSubscription(callCtx, &subscriptionsv1.SetSubscriptionRequest{ChatId: chatID, StreetId: state.street.Id, Building: text})
	cancel()
	if err != nil {
		return nil, err
	}
	delete(b.pending, chatID)
	return &reply{text: fmt.Sprintf("Ви підписалися на сповіщення про відключення електроенергії для вулиці %s, будинок %s.", result.GetAddress().GetStreet().GetName(), result.GetAddress().GetBuilding()), removeKeyboard: true}, nil
}

// command accepts Telegram's /name@bot_name suffix and trailing arguments.
func command(text string) string {
	if !strings.HasPrefix(text, "/") {
		return ""
	}
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return ""
	}
	name := strings.TrimPrefix(parts[0], "/")
	if i := strings.IndexByte(name, '@'); i >= 0 {
		for _, c := range name[i+1:] {
			if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_') {
				return ""
			}
		}
		if i+1 == len(name) {
			return ""
		}
		name = name[:i]
	}
	return name
}
