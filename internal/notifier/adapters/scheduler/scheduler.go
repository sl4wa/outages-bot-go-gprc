package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"outages-bot/internal/notifier/application/core/domain"
	"outages-bot/internal/notifier/ports"
)

type Adapter struct {
	api ports.APIPort
}

func NewAdapter(api ports.APIPort) *Adapter { return &Adapter{api: api} }

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
		if _, err := a.api.RunOnce(ctx); err != nil && ctx.Err() == nil && !errors.Is(err, domain.ErrRunInProgress) {
			slog.ErrorContext(ctx, "automatic notification run failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
