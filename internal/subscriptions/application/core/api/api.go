package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"

	"outages-bot/internal/subscriptions/application/core/domain"
	"outages-bot/internal/subscriptions/ports"
)

// Application serializes complete read/check/write operations, including guards.
// A single application instance owns the subscription repository.
type Application struct {
	db      ports.DBPort
	mu      sync.RWMutex
	streets []domain.Street
	byID    map[int32]domain.Street
}

var _ ports.APIPort = (*Application)(nil)

func NewApplication(db ports.DBPort, streets []domain.Street) *Application {
	a := &Application{db: db, streets: append([]domain.Street(nil), streets...), byID: make(map[int32]domain.Street)}
	for _, street := range streets {
		a.byID[street.ID] = street
	}
	return a
}

func (a *Application) SearchStreets(ctx context.Context, query string) (domain.StreetSearch, error) {
	if err := ctx.Err(); err != nil {
		return domain.StreetSearch{}, err
	}
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return domain.StreetSearch{}, invalidField("query", "street query is required")
	}
	result := domain.StreetSearch{}
	for _, street := range a.streets {
		name := strings.ToLower(street.Name)
		if name == query {
			return domain.StreetSearch{Streets: []domain.Street{street}}, nil
		}
		if strings.Contains(name, query) {
			if len(result.Streets) < 30 {
				result.Streets = append(result.Streets, street)
			} else {
				result.TooMany = true
			}
		}
	}
	if result.TooMany {
		result.Streets = nil
	}
	return result, nil
}

func (a *Application) GetSubscription(ctx context.Context, chatID int64) (domain.Subscription, error) {
	if err := validChat(chatID); err != nil {
		return domain.Subscription{}, err
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.db.Get(ctx, chatID)
}

func (a *Application) SetSubscription(ctx context.Context, chatID int64, streetID int32, building string) (domain.Subscription, error) {
	if err := validChat(chatID); err != nil {
		return domain.Subscription{}, err
	}
	street, exists := a.byID[streetID]
	if !exists {
		return domain.Subscription{}, invalidField("street_id", "unknown street ID")
	}
	if strings.TrimSpace(building) == "" {
		return domain.Subscription{}, invalidField("building", "building is required")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return domain.Subscription{}, err
	}
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return domain.Subscription{}, fmt.Errorf("cannot generate subscription identity")
	}
	subscription := domain.Subscription{
		ChatID: chatID, Address: domain.Address{Street: street, Building: building},
		Generation: hex.EncodeToString(token[:]),
	}
	if err := a.db.Save(ctx, subscription); err != nil {
		return domain.Subscription{}, err
	}
	return subscription, nil
}

func (a *Application) DeleteSubscription(ctx context.Context, chatID int64, expected *domain.SubscriptionGuard) (bool, error) {
	if err := validChat(chatID); err != nil {
		return false, err
	}
	if expected != nil {
		if err := validGuard(*expected); err != nil {
			return false, err
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	subscription, err := a.db.Get(ctx, chatID)
	if errors.Is(err, domain.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if expected != nil && subscription.Generation != expected.Generation {
		return false, nil
	}
	if err := a.db.Delete(ctx, chatID); err != nil {
		return false, err
	}
	return true, nil
}

func (a *Application) ListSubscriptions(ctx context.Context) ([]domain.Subscription, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.db.List(ctx)
}

func (a *Application) MarkNotified(ctx context.Context, chatID int64, expected domain.SubscriptionGuard, notification *domain.Notification) (bool, error) {
	if err := validChat(chatID); err != nil {
		return false, err
	}
	if err := validGuard(expected); err != nil {
		return false, err
	}
	if notification == nil {
		return false, invalidField("notification", "notification is required")
	}
	if strings.TrimSpace(notification.StartDate) == "" {
		return false, invalidField("notification.start_date", "start date is required")
	}
	if strings.TrimSpace(notification.EndDate) == "" {
		return false, invalidField("notification.end_date", "end date is required")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	subscription, err := a.db.Get(ctx, chatID)
	if errors.Is(err, domain.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if subscription.Generation != expected.Generation {
		return false, nil
	}
	receipt := *notification
	subscription.LastNotification = &receipt
	if err := a.db.Save(ctx, subscription); err != nil {
		return false, err
	}
	return true, nil
}

func validChat(id int64) error {
	if id == 0 {
		return invalidField("chat_id", "chat ID must not be zero")
	}
	return nil
}

func validGuard(guard domain.SubscriptionGuard) error {
	if strings.TrimSpace(guard.Generation) == "" {
		return invalidField("expected.generation", "subscription generation is required")
	}
	return nil
}

func invalidField(field, description string) error {
	return &domain.ValidationError{Field: field, Description: description}
}
