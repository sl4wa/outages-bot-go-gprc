// Package db persists subscriber records as JSON. Only one process may own a directory.
package db

import (
	"context"
	"fmt"
	"os"
	"sort"
	"sync"

	"outages-bot/internal/subscriptions/application/core/domain"
	"outages-bot/internal/subscriptions/ports"
)

type Adapter struct {
	mu       sync.RWMutex
	usersDir string
	records  map[int64]record
}

var _ ports.DBPort = (*Adapter)(nil)

func NewAdapter(usersDir string) (*Adapter, error) {
	if usersDir == "" {
		return nil, fmt.Errorf("users directory is required")
	}
	if err := os.MkdirAll(usersDir, 0700); err != nil {
		return nil, fmt.Errorf("create users directory: %w", err)
	}
	a := &Adapter{usersDir: usersDir, records: make(map[int64]record)}
	if err := a.loadRecords(); err != nil {
		return nil, fmt.Errorf("load subscriptions: %w", err)
	}
	return a, nil
}

func (a *Adapter) Get(ctx context.Context, id int64) (domain.Subscription, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return domain.Subscription{}, err
	}
	r, exists := a.records[id]
	if !exists {
		return domain.Subscription{}, domain.ErrNotFound
	}
	return r.subscription(id), nil
}

func (a *Adapter) Save(ctx context.Context, subscription domain.Subscription) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	r := recordFrom(subscription)
	if err := a.save(subscription.ChatID, &r); err != nil {
		return err
	}
	a.records[subscription.ChatID] = r
	return nil
}

func (a *Adapter) Delete(ctx context.Context, id int64) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Remove(a.path(id)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cannot delete subscription")
	}
	delete(a.records, id)
	return nil
}

func (a *Adapter) List(ctx context.Context) ([]domain.Subscription, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := make([]domain.Subscription, 0, len(a.records))
	for id, r := range a.records {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		result = append(result, r.subscription(id))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ChatID < result[j].ChatID })
	return result, nil
}
