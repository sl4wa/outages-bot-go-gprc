package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	notificationsv1 "outages-bot/gen/notifications/v1"
	outagesv1 "outages-bot/gen/outages/v1"
	subscriptionsv1 "outages-bot/gen/subscriptions/v1"
	"outages-bot/internal/notifier"
	"outages-bot/internal/platform/config"
	"outages-bot/internal/platform/rpc"
	"outages-bot/internal/platform/telegram"
)

func main() {
	if err := run(); err != nil {
		slog.Error("notifier stopped", "error", err)
		os.Exit(1)
	}
}
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	interval, err := config.Duration("NOTIFY_INTERVAL", time.Minute)
	if err != nil {
		return err
	}
	transport, err := telegram.New(os.Getenv("TELEGRAM_BOT_TOKEN"), config.String("TELEGRAM_API_URL", "https://api.telegram.org"), nil)
	if err != nil {
		return err
	}
	subConn, err := rpc.Dial(config.String("SUBSCRIPTIONS_ADDR", "127.0.0.1:50051"))
	if err != nil {
		return err
	}
	defer subConn.Close()
	outageConn, err := rpc.Dial(config.String("OUTAGES_ADDR", "127.0.0.1:50052"))
	if err != nil {
		return err
	}
	defer outageConn.Close()
	service := notifier.New(subscriptionsv1.NewSubscriptionServiceClient(subConn), outagesv1.NewOutageServiceClient(outageConn), transport)
	server := rpc.NewServer()
	notificationsv1.RegisterNotificationServiceServer(server, service)
	listener, err := net.Listen("tcp", config.String("NOTIFIER_LISTEN", "127.0.0.1:50053"))
	if err != nil {
		return err
	}
	loopDone := make(chan struct{})
	go func() { defer close(loopDone); service.Run(ctx, interval) }()
	err = rpc.ServeListener(ctx, listener, server)
	stop()
	// Canceled runs finish any in-flight delivery acknowledgment before exit.
	<-loopDone
	return err
}
