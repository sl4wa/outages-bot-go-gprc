package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	outagesv1 "outages-bot/gen/outages/v1"
	"outages-bot/internal/outages"
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
	service := outages.New(config.String("OUTAGE_API_URL", "https://power-api.loe.lviv.ua/api/pw_accidents?pagination=false&otg.id=28&city.id=693"), nil)
	server := rpc.NewServer()
	outagesv1.RegisterOutageServiceServer(server, service)
	return rpc.Serve(ctx, config.String("OUTAGES_LISTEN", "127.0.0.1:50052"), server)
}
