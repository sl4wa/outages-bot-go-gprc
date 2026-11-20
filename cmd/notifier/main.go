package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // the runtime image has no zoneinfo

	notificationsv1 "outages-bot/gen/notifications/v1"
	outagesv1 "outages-bot/gen/outages/v1"
	schedulesv1 "outages-bot/gen/schedules/v1"
	subscriptionsv1 "outages-bot/gen/subscriptions/v1"
	grpcadapter "outages-bot/internal/notifier/adapters/grpc"
	"outages-bot/internal/notifier/adapters/outages"
	"outages-bot/internal/notifier/adapters/scheduler"
	schedulesadapter "outages-bot/internal/notifier/adapters/schedules"
	"outages-bot/internal/notifier/adapters/subscriptions"
	telegramadapter "outages-bot/internal/notifier/adapters/telegram"
	"outages-bot/internal/notifier/application/core/api"
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
	interval, err := config.Duration("NOTIFY_INTERVAL")
	if err != nil {
		return err
	}
	apiURL, err := config.String("TELEGRAM_API_URL")
	if err != nil {
		return err
	}
	subscriptionsAddr, err := config.String("SUBSCRIPTIONS_ADDR")
	if err != nil {
		return err
	}
	outagesAddr, err := config.String("OUTAGES_ADDR")
	if err != nil {
		return err
	}
	schedulesAddr, err := config.String("SCHEDULES_ADDR")
	if err != nil {
		return err
	}
	notifierListen, err := config.String("NOTIFIER_LISTEN")
	if err != nil {
		return err
	}
	transport, err := telegram.New(os.Getenv("TELEGRAM_BOT_TOKEN"), apiURL, nil)
	if err != nil {
		return err
	}
	subConn, err := rpc.Dial(subscriptionsAddr)
	if err != nil {
		return err
	}
	defer subConn.Close()
	outageConn, err := rpc.Dial(outagesAddr)
	if err != nil {
		return err
	}
	defer outageConn.Close()
	scheduleConn, err := rpc.Dial(schedulesAddr)
	if err != nil {
		return err
	}
	defer scheduleConn.Close()
	subscriptionAdapter := subscriptions.NewAdapter(subscriptionsv1.NewSubscriptionServiceClient(subConn))
	outageAdapter := outages.NewAdapter(outagesv1.NewOutageServiceClient(outageConn))
	senderAdapter := telegramadapter.NewAdapter(transport)
	application := api.NewApplication(subscriptionAdapter, outageAdapter, senderAdapter)
	grpcAdapter := grpcadapter.NewAdapter(application)
	outageJob := scheduler.NewAdapter("outages", func(ctx context.Context) error { _, err := application.RunOnce(ctx); return err })
	server := rpc.NewServer()
	notificationsv1.RegisterNotificationServiceServer(server, grpcAdapter)
	listener, err := net.Listen("tcp", notifierListen)
	if err != nil {
		return err
	}
	scheduleInterval, err := config.Duration("SCHEDULE_INTERVAL")
	if err != nil {
		return err
	}
	kyiv, err := time.LoadLocation("Europe/Kyiv")
	if err != nil {
		return err
	}
	scheduleApplication := api.NewScheduleApplication(schedulesadapter.NewAdapter(schedulesv1.NewScheduleServiceClient(scheduleConn)), subscriptionAdapter, senderAdapter, kyiv)
	scheduleJob := scheduler.NewAdapter("schedules", scheduleApplication.CheckSchedules)
	loopDone := make(chan struct{})
	scheduleDone := make(chan struct{})
	go func() { defer close(loopDone); outageJob.Run(ctx, interval) }()
	go func() { defer close(scheduleDone); scheduleJob.Run(ctx, scheduleInterval) }()
	err = rpc.ServeListener(ctx, listener, server)
	stop()
	// Canceled runs finish any in-flight delivery acknowledgment before exit.
	<-loopDone
	<-scheduleDone
	return err
}
