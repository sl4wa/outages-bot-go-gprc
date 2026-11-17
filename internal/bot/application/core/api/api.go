package api

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"outages-bot/internal/bot/application/core/domain"
	"outages-bot/internal/bot/ports"
)

const (
	rpcTimeout = 5 * time.Second
	pendingTTL = 30 * time.Minute
)

// Application owns conversations. Its caller processes messages sequentially.
// Durable subscriptions are accessed through the subscription port.
type Application struct {
	subscriptions ports.SubscriptionPort
	pending       map[int64]conversation
	now           func() time.Time
}

var _ ports.APIPort = (*Application)(nil)

func NewApplication(subscriptions ports.SubscriptionPort) *Application {
	return &Application{subscriptions: subscriptions, pending: make(map[int64]conversation), now: time.Now}
}

func (a *Application) ExpireConversations() {
	for chatID, state := range a.pending {
		if a.now().Sub(state.started) > pendingTTL {
			delete(a.pending, chatID)
		}
	}
}

type conversationStep uint8

const (
	searchStreet conversationStep = iota
	enterBuilding
)

type conversation struct {
	step    conversationStep
	street  domain.Street
	started time.Time
}

func (a *Application) HandleText(ctx context.Context, chatID int64, text string) (*domain.Reply, error) {
	switch command(text) {
	case "start":
		callCtx, cancel := context.WithTimeout(ctx, rpcTimeout)
		current, err := a.subscriptions.Get(callCtx, chatID)
		cancel()
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return nil, err
		}
		a.pending[chatID] = conversation{step: searchStreet, started: a.now()}
		if err == nil {
			return &domain.Reply{Text: fmt.Sprintf("Ваша поточна підписка:\nВулиця: %s\nБудинок: %s\n\nБудь ласка, введіть нову назву вулиці для оновлення підписки:", current.Address.Street.Name, current.Address.Building), RemoveKeyboard: true}, nil
		}
		return &domain.Reply{Text: "Будь ласка, введіть назву вулиці:", RemoveKeyboard: true}, nil
	case "subscription":
		callCtx, cancel := context.WithTimeout(ctx, rpcTimeout)
		defer cancel()
		current, err := a.subscriptions.Get(callCtx, chatID)
		if errors.Is(err, domain.ErrNotFound) {
			return &domain.Reply{Text: "Ви не маєте активної підписки."}, nil
		}
		if err != nil {
			return nil, err
		}
		return &domain.Reply{Text: fmt.Sprintf("Ваша поточна підписка:\nВулиця: %s\nБудинок: %s", current.Address.Street.Name, current.Address.Building)}, nil
	case "stop":
		callCtx, cancel := context.WithTimeout(ctx, rpcTimeout)
		result, err := a.subscriptions.Delete(callCtx, chatID)
		cancel()
		if err != nil {
			return nil, err
		}
		delete(a.pending, chatID)
		if result {
			return &domain.Reply{Text: "Ви успішно відписалися від сповіщень про відключення електроенергії.", RemoveKeyboard: true}, nil
		}
		return &domain.Reply{Text: "Ви не маєте активної підписки.", RemoveKeyboard: true}, nil
	}
	state, ok := a.pending[chatID]
	if !ok {
		return nil, nil
	}
	switch state.step {
	case searchStreet:
		return a.searchStreet(ctx, chatID, text, state)
	case enterBuilding:
		return a.saveSubscription(ctx, chatID, text, state)
	default:
		return nil, fmt.Errorf("unknown conversation step %d", state.step)
	}
}

func (a *Application) searchStreet(ctx context.Context, chatID int64, text string, state conversation) (*domain.Reply, error) {
	query := strings.TrimSpace(text)
	if query == "" {
		return &domain.Reply{Text: "Введіть назву вулиці."}, nil
	}
	callCtx, cancel := context.WithTimeout(ctx, rpcTimeout)
	result, err := a.subscriptions.SearchStreets(callCtx, query)
	cancel()
	if err != nil {
		return nil, err
	}
	if result.TooMany {
		return &domain.Reply{Text: "Знайдено забагато вулиць. Введіть точнішу назву:"}, nil
	}
	switch len(result.Streets) {
	case 0:
		return &domain.Reply{Text: "Вулицю не знайдено. Спробуйте ще раз."}, nil
	case 1:
		street := result.Streets[0]
		state.step, state.street = enterBuilding, street
		a.pending[chatID] = state
		return &domain.Reply{Text: fmt.Sprintf("Ви обрали вулицю: %s\nБудь ласка, введіть номер будинку:", street.Name), RemoveKeyboard: true}, nil
	default:
		keyboard := make([][]string, len(result.Streets))
		for i, street := range result.Streets {
			keyboard[i] = []string{street.Name}
		}
		return &domain.Reply{Text: "Будь ласка, оберіть вулицю:", Keyboard: keyboard}, nil
	}
}

func (a *Application) saveSubscription(ctx context.Context, chatID int64, text string, state conversation) (*domain.Reply, error) {
	if strings.TrimSpace(text) == "" {
		return &domain.Reply{Text: "номер будинку не може бути порожнім"}, nil
	}
	callCtx, cancel := context.WithTimeout(ctx, rpcTimeout)
	result, err := a.subscriptions.Set(callCtx, chatID, state.street.ID, text)
	cancel()
	if err != nil {
		return nil, err
	}
	delete(a.pending, chatID)
	return &domain.Reply{Text: fmt.Sprintf("Ви підписалися на сповіщення про відключення електроенергії для вулиці %s, будинок %s.", result.Address.Street.Name, result.Address.Building), RemoveKeyboard: true}, nil
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
