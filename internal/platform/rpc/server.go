package rpc

import (
	"context"
	"log/slog"
	"net"
	"runtime/debug"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

func Dial(target string) (*grpc.ClientConn, error) {
	return grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
}

func NewServer() *grpc.Server {
	return grpc.NewServer(grpc.ChainUnaryInterceptor(unaryLog), grpc.ChainStreamInterceptor(streamLog))
}

func Serve(ctx context.Context, address string, server *grpc.Server) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	return ServeListener(ctx, listener, server)
}

// ServeListener takes ownership of a bound listener. Processes with background
// side effects can bind successfully before starting their workers.
func ServeListener(ctx context.Context, listener net.Listener, server *grpc.Server) error {
	defer listener.Close()
	h := health.NewServer()
	healthpb.RegisterHealthServer(server, h)
	reflection.Register(server)
	for name := range server.GetServiceInfo() {
		h.SetServingStatus(name, healthpb.HealthCheckResponse_SERVING)
	}
	h.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	finished := make(chan error, 1)
	go func() { finished <- server.Serve(listener) }()
	slog.Info("grpc listening", "address", listener.Addr().String())
	select {
	case err := <-finished:
		return err
	case <-ctx.Done():
		h.Shutdown()
		stopped := make(chan struct{})
		go func() { server.GracefulStop(); close(stopped) }()
		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()
		select {
		case <-stopped:
		case <-timer.C:
			server.Stop()
			<-stopped
		}
		return <-finished
	}
}

func unaryLog(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
	start := time.Now()
	defer finishRPC(ctx, info.FullMethod, start, &err)
	return handler(ctx, req)
}

func streamLog(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
	start := time.Now()
	defer finishRPC(stream.Context(), info.FullMethod, start, &err)
	return handler(srv, stream)
}

func finishRPC(ctx context.Context, method string, start time.Time, err *error) {
	if recover() != nil {
		// The stack identifies the fault; the panic value may contain user data.
		slog.ErrorContext(ctx, "rpc panic", "method", method, "stack", string(debug.Stack()))
		*err = status.Error(codes.Internal, "internal server error")
	}
	slog.InfoContext(ctx, "rpc", "method", method, "code", status.Code(*err).String(), "duration", time.Since(start))
}
