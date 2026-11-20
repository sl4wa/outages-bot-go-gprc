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
	feedAdapter := loe.NewAdapter(config.String("SCHEDULE_API_URL", "https://api.loe.lviv.ua/api/menus?page=1&type=photo-grafic"), nil)
	application := api.NewApplication(feedAdapter)
	grpcAdapter := grpcadapter.NewAdapter(application)
	server := rpc.NewServer()
	schedulesv1.RegisterScheduleServiceServer(server, grpcAdapter)
	return rpc.Serve(ctx, config.String("SCHEDULES_LISTEN", "127.0.0.1:50054"), server)
}
