package api

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"time"

	"outages-bot/internal/notifier/application/core/domain"
)

// CheckSchedules broadcasts changes to today's and tomorrow's schedules.
// Like the C notifier, it records what it sent before delivering and does not
// retry failed sends. State is in memory, so the first check after a start
// only records the current schedules. Only the schedule job calls it.
func (a *Application) CheckSchedules(ctx context.Context) error {
	fetchCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	published, err := a.schedules.FetchSchedules(fetchCtx)
	if err != nil {
		return err
	}
	now := time.Now().In(a.loc)
	today := now.Format(time.DateOnly)
	var selected []domain.Schedule
	changed := false
	for _, day := range []string{today, now.AddDate(0, 0, 1).Format(time.DateOnly)} {
		if s := domain.LatestFor(published, day); s != nil {
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
func (a *Application) broadcast(ctx context.Context, today string, schedules []domain.Schedule) error {
	listCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	subscribers, err := a.subscriptions.List(listCtx)
	cancel()
	if err != nil {
		return err
	}
	for _, subscriber := range subscribers {
		sendCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		err := a.sender.SendSchedules(sendCtx, subscriber.ChatID, today, schedules)
		cancel()
		if errors.Is(err, domain.ErrBlocked) {
			deleteCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			_, err = a.subscriptions.Delete(deleteCtx, subscriber)
			cancel()
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
