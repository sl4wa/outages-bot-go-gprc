package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	schedulesv1 "outages-bot/gen/schedules/v1"
	"outages-bot/internal/platform/config"
	"outages-bot/internal/platform/rpc"
	grpcadapter "outages-bot/internal/schedules/adapters/grpc"
	"outages-bot/internal/schedules/adapters/loe"
	"outages-bot/internal/schedules/application/core/api"
)

func main() {
	if err := run(); err != nil {
		slog.Error("schedules stopped", "error", err)
		os.Exit(1)
	}
}
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	feedURL, err := config.String("SCHEDULE_API_URL")
	if err != nil {
		return err
	}
	listen, err := config.String("SCHEDULES_LISTEN")
	if err != nil {
		return err
	}
	feedAdapter := loe.NewAdapter(feedURL, nil)
	application := api.NewApplication(feedAdapter)
	grpcAdapter := grpcadapter.NewAdapter(application)
	server := rpc.NewServer()
	schedulesv1.RegisterScheduleServiceServer(server, grpcAdapter)
	return rpc.Serve(ctx, listen, server)
}
