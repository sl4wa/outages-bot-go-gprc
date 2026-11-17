package grpc

import (
	"context"
	"errors"

	pb "outages-bot/gen/notifications/v1"
	"outages-bot/internal/notifier/application/core/domain"
	"outages-bot/internal/notifier/ports"
	"outages-bot/internal/platform/rpc"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
)

type Adapter struct {
	pb.UnimplementedNotificationServiceServer
	api ports.APIPort
}

func NewAdapter(api ports.APIPort) *Adapter { return &Adapter{api: api} }

func (a *Adapter) RunOnce(ctx context.Context, _ *pb.RunOnceRequest) (*pb.RunOnceResponse, error) {
	result, err := a.api.RunOnce(ctx)
	if err != nil {
		return nil, toStatus(err)
	}
	return &pb.RunOnceResponse{
		Subscribers: result.Subscribers, Matched: result.Matched, Sent: result.Sent,
		Skipped: result.Skipped, Unsubscribed: result.Unsubscribed,
	}, nil
}

func toStatus(err error) error {
	if errors.Is(err, domain.ErrRunInProgress) {
		return status.Error(codes.FailedPrecondition, err.Error())
	}
	var rateLimit *domain.RateLimitError
	if errors.As(err, &rateLimit) {
		s := status.New(codes.ResourceExhausted, rateLimit.Error())
		if rateLimit.RetryAfter > 0 {
			// Use seconds directly so a remote integer cannot overflow time.Duration.
			delay := &durationpb.Duration{Seconds: int64(rateLimit.RetryAfter)}
			if delay.CheckValid() == nil {
				if withDetails, err := s.WithDetails(&errdetails.RetryInfo{RetryDelay: delay}); err == nil {
					s = withDetails
				}
			}
		}
		return s.Err()
	}
	return rpc.DependencyError("", err)
}
