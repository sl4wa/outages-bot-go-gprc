package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	subscriptionsv1 "outages-bot/gen/subscriptions/v1"
	"outages-bot/internal/platform/config"
	"outages-bot/internal/platform/rpc"
	"outages-bot/internal/subscriptions/adapters/catalog"
	"outages-bot/internal/subscriptions/adapters/db"
	grpcadapter "outages-bot/internal/subscriptions/adapters/grpc"
	"outages-bot/internal/subscriptions/application/core/api"
)

func main() {
	if err := run(); err != nil {
		slog.Error("subscriptions stopped", "error", err)
		os.Exit(1)
	}
}
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	usersDir, err := config.String("USERS_DIR")
	if err != nil {
		return err
	}
	listen, err := config.String("SUBSCRIPTIONS_LISTEN")
	if err != nil {
		return err
	}
	dbAdapter, err := db.NewAdapter(usersDir)
	if err != nil {
		return err
	}
	streets, err := catalog.Load()
	if err != nil {
		return err
	}
	application := api.NewApplication(dbAdapter, streets)
	grpcAdapter := grpcadapter.NewAdapter(application)
	server := rpc.NewServer()
	subscriptionsv1.RegisterSubscriptionServiceServer(server, grpcAdapter)
	return rpc.Serve(ctx, listen, server)
}
