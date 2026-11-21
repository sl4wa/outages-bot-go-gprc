package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	subscriptionsv1 "outages-bot/gen/subscriptions/v1"
	"outages-bot/internal/bot/adapters/subscriptions"
	telegramadapter "outages-bot/internal/bot/adapters/telegram"
	"outages-bot/internal/bot/application/core/api"
	"outages-bot/internal/platform/config"
	"outages-bot/internal/platform/rpc"
	"outages-bot/internal/platform/telegram"
)

func main() {
	if err := run(); err != nil {
		slog.Error("bot stopped", "error", err)
		os.Exit(1)
	}
}
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	apiURL, err := config.String("TELEGRAM_API_URL")
	if err != nil {
		return err
	}
	subscriptionsAddr, err := config.String("SUBSCRIPTIONS_ADDR")
	if err != nil {
		return err
	}
	transport, err := telegram.New(os.Getenv("TELEGRAM_BOT_TOKEN"), apiURL, nil)
	if err != nil {
		return err
	}
	// No retries: SetSubscription and unguarded deletes are not safe to repeat.
	conn, err := rpc.Dial(subscriptionsAddr)
	if err != nil {
		return err
	}
	defer conn.Close()
	subscriptionAdapter := subscriptions.NewAdapter(subscriptionsv1.NewSubscriptionServiceClient(conn))
	application := api.NewApplication(subscriptionAdapter)
	telegramAdapter := telegramadapter.NewAdapter(application, transport)
	return telegramAdapter.Run(ctx)
}
