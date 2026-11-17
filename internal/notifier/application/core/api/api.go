package api

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"outages-bot/internal/notifier/application/core/domain"
	"outages-bot/internal/notifier/ports"
)

type Application struct {
	subscriptions ports.SubscriptionPort
	outages       ports.OutagePort
	sender        ports.SenderPort
	run           sync.Mutex
}

var _ ports.APIPort = (*Application)(nil)

func NewApplication(subscriptions ports.SubscriptionPort, outages ports.OutagePort, sender ports.SenderPort) *Application {
	return &Application{subscriptions: subscriptions, outages: outages, sender: sender}
}

func (a *Application) RunOnce(ctx context.Context) (*domain.RunResult, error) {
	if !a.run.TryLock() {
		return nil, domain.ErrRunInProgress
	}
	defer a.run.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	result := &domain.RunResult{}
	defer func() {
		slog.InfoContext(ctx, "notification run finished", "subscribers", result.Subscribers, "matched", result.Matched,
			"sent", result.Sent, "skipped", result.Skipped, "unsubscribed", result.Unsubscribed)
	}()

	fetchCtx, fetchCancel := context.WithTimeout(ctx, 25*time.Second)
	snapshot, err := a.outages.FetchSnapshot(fetchCtx)
	fetchCancel()
	if err != nil {
		return nil, err
	}
	// Collect the complete subscription snapshot before sending any messages.
	listCtx, listCancel := context.WithTimeout(ctx, 5*time.Second)
	subscribers, err := a.subscriptions.List(listCtx)
	listCancel()
	if err != nil {
		return nil, err
	}
	result.Subscribers = int32(len(subscribers))
	for _, subscriber := range subscribers {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		outage := domain.MatchingOutage(subscriber, snapshot)
		if outage == nil {
			result.Skipped++
			continue
		}
		result.Matched++
		sendCtx, sendCancel := context.WithTimeout(ctx, 20*time.Second)
		err := a.sender.Send(sendCtx, subscriber.ChatID, *outage)
		sendCancel()
		if err != nil {
			var rateLimit *domain.RateLimitError
			switch {
			case errors.Is(err, domain.ErrRejected):
				result.Skipped++
				continue
			case errors.Is(err, domain.ErrBlocked):
				deleteCtx, deleteCancel := context.WithTimeout(ctx, 5*time.Second)
				deleted, deleteErr := a.subscriptions.Delete(deleteCtx, subscriber)
				deleteCancel()
				if deleteErr != nil {
					return nil, deleteErr
				}
				if deleted {
					result.Unsubscribed++
				} else {
					result.Skipped++
				}
				continue
			case errors.As(err, &rateLimit):
				return nil, err
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return nil, domain.ErrDelivery
		}
		result.Sent++
		// After acceptance, make one bounded attempt to record delivery even if the
		// caller canceled. A crash between sending and recording can still duplicate.
		ackCtx, ackCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		err = a.subscriptions.MarkNotified(ackCtx, subscriber, domain.Notification{
			StartDate: outage.StartDate, EndDate: outage.EndDate, Comment: outage.Comment,
		})
		ackCancel()
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}
