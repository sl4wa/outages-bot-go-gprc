package api

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"time"

	"outages-bot/internal/notifier/application/core/domain"
	"outages-bot/internal/notifier/ports"
)

// ScheduleApplication broadcasts changes to today's and tomorrow's schedules.
// Like the C notifier, it records what it sent before delivering and does not
// retry failed sends. State is in memory, so the first check after a start
// only records the current schedules.
type ScheduleApplication struct {
	feed          ports.ScheduleFeedPort
	subscriptions ports.SubscriptionPort
	sender        ports.ScheduleSenderPort
	loc           *time.Location // the provider's timezone, which defines "today"

	sent   map[string]domain.Schedule // day -> last broadcast schedule
	seeded bool
}

func NewScheduleApplication(feed ports.ScheduleFeedPort, subscriptions ports.SubscriptionPort, sender ports.ScheduleSenderPort, loc *time.Location) *ScheduleApplication {
	return &ScheduleApplication{feed: feed, subscriptions: subscriptions, sender: sender, loc: loc,
		sent: map[string]domain.Schedule{}}
}

func (a *ScheduleApplication) CheckSchedules(ctx context.Context) error {
	fetchCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	schedules, err := a.feed.FetchSchedules(fetchCtx)
	if err != nil {
		return err
	}
	now := time.Now().In(a.loc)
	today := now.Format(time.DateOnly)
	var selected []domain.Schedule
	changed := false
	for _, day := range []string{today, now.AddDate(0, 0, 1).Format(time.DateOnly)} {
		if s := domain.LatestFor(schedules, day); s != nil {
			selected = append(selected, *s)
			if previous, ok := a.sent[day]; !ok || !slices.Equal(previous.Lines, s.Lines) {
				changed = true
			}
		}
	}
	// Update only the days present, so a feed that briefly omits a day does not
	// make its unchanged schedule look new later.
	for _, s := range selected {
		a.sent[s.Date] = s
	}
	for day := range a.sent {
		if day < today {
			delete(a.sent, day)
		}
	}
	first := !a.seeded
	a.seeded = true
	if first || !changed {
		return nil
	}
	return a.broadcast(ctx, today, selected)
}

// broadcast tries every subscriber once; failures are logged, not retried.
func (a *ScheduleApplication) broadcast(ctx context.Context, today string, schedules []domain.Schedule) error {
	subscribers, err := a.subscriptions.List(ctx)
	if err != nil {
		return err
	}
	for _, subscriber := range subscribers {
		err := a.sender.SendSchedules(ctx, subscriber.ChatID, today, schedules)
		if errors.Is(err, domain.ErrBlocked) {
			_, err = a.subscriptions.Delete(ctx, subscriber)
		}
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			slog.WarnContext(ctx, "schedule delivery failed", "chat_id", subscriber.ChatID, "error", err)
		}
	}
	return nil
}
