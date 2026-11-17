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
	transport, err := telegram.New(os.Getenv("TELEGRAM_BOT_TOKEN"), config.String("TELEGRAM_API_URL", "https://api.telegram.org"), nil)
	if err != nil {
		return err
	}
	conn, err := rpc.Dial(config.String("SUBSCRIPTIONS_ADDR", "127.0.0.1:50051"))
	if err != nil {
		return err
	}
	defer conn.Close()
	subscriptionAdapter := subscriptions.NewAdapter(subscriptionsv1.NewSubscriptionServiceClient(conn))
	application := api.NewApplication(subscriptionAdapter)
	telegramAdapter := telegramadapter.NewAdapter(application, transport)
	return telegramAdapter.Run(ctx)
}
