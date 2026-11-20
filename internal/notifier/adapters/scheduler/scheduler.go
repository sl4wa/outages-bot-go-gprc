package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"outages-bot/internal/notifier/application/core/domain"
)

// Adapter is a driving adapter that invokes a job on a fixed interval.
type Adapter struct {
	name string
	job  func(context.Context) error
}

func NewAdapter(name string, job func(context.Context) error) *Adapter {
	return &Adapter{name: name, job: job}
}

// Run starts immediately, then repeats until cancellation. Zero disables it.
func (a *Adapter) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		if err := a.job(ctx); err != nil && ctx.Err() == nil && !errors.Is(err, domain.ErrRunInProgress) {
			slog.ErrorContext(ctx, "automatic run failed", "job", a.name, "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
