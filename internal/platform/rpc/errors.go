package rpc

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// DependencyError retains downstream status codes and rich details at an adapter boundary.
func DependencyError(operation string, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		err = status.FromContextError(err).Err()
	}
	// Status.Proto returns a copy, retaining rich protobuf error details while
	// adding the operation that failed at this service boundary.
	s := status.Convert(err).Proto()
	if codes.Code(s.Code) == codes.Unknown {
		s.Code = int32(codes.Unavailable)
	}
	if operation != "" {
		s.Message = operation + ": " + s.Message
	}
	return status.FromProto(s).Err()
}
