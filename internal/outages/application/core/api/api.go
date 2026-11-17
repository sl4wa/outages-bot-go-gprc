package api

import (
	"context"
	"time"

	"outages-bot/internal/outages/application/core/domain"
	"outages-bot/internal/outages/ports"
)

type Application struct {
	feed ports.FeedPort
}

var _ ports.APIPort = (*Application)(nil)

func NewApplication(feed ports.FeedPort) *Application { return &Application{feed: feed} }

// FetchSnapshot obtains a fresh snapshot, bounded by the caller's deadline.
func (a *Application) FetchSnapshot(ctx context.Context) ([]domain.Outage, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return a.feed.Fetch(ctx)
}
