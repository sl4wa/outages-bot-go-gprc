package api

import (
	"context"
	"time"

	"outages-bot/internal/schedules/application/core/domain"
	"outages-bot/internal/schedules/ports"
)

type Application struct {
	feed ports.FeedPort
}

var _ ports.APIPort = (*Application)(nil)

func NewApplication(feed ports.FeedPort) *Application { return &Application{feed: feed} }

// FetchSchedules obtains the published schedules, bounded by the caller's deadline.
func (a *Application) FetchSchedules(ctx context.Context) ([]domain.Schedule, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return a.feed.Fetch(ctx)
}
