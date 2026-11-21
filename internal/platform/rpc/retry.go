package rpc

import (
	"time"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/retry"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

// Retry returns dial options that retry calls failing with UNAVAILABLE, such as
// a dependency that is restarting or an upstream feed that briefly failed.
// Use them only on connections whose calls are all safe to repeat.
// Attempts share the caller's deadline; streams retry only before the first message.
func Retry() []grpc.DialOption {
	policy := []retry.CallOption{
		retry.WithCodes(codes.Unavailable),
		retry.WithMax(3),
		retry.WithBackoff(retry.BackoffExponentialWithJitter(500*time.Millisecond, 0.2)),
	}
	return []grpc.DialOption{
		grpc.WithChainUnaryInterceptor(retry.UnaryClientInterceptor(policy...)),
		grpc.WithChainStreamInterceptor(retry.StreamClientInterceptor(policy...)),
	}
}
