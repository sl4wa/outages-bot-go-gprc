package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	outagesv1 "outages-bot/gen/outages/v1"
	grpcadapter "outages-bot/internal/outages/adapters/grpc"
	"outages-bot/internal/outages/adapters/loe"
	"outages-bot/internal/outages/application/core/api"
	"outages-bot/internal/platform/config"
	"outages-bot/internal/platform/rpc"
)

func main() {
	if err := run(); err != nil {
		slog.Error("outages stopped", "error", err)
		os.Exit(1)
	}
}
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	feedURL, err := config.String("OUTAGE_API_URL")
	if err != nil {
		return err
	}
	listen, err := config.String("OUTAGES_LISTEN")
	if err != nil {
		return err
	}
	feedAdapter := loe.NewAdapter(feedURL, nil)
	application := api.NewApplication(feedAdapter)
	grpcAdapter := grpcadapter.NewAdapter(application)
	server := rpc.NewServer()
	outagesv1.RegisterOutageServiceServer(server, grpcAdapter)
	return rpc.Serve(ctx, listen, server)
}
