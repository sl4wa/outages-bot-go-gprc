package grpc

import (
	"context"
	"errors"

	"outages-bot/internal/subscriptions/application/core/domain"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func toStatus(err error) error {
	var validation *domain.ValidationError
	var dataLoss *domain.DataLossError
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return status.FromContextError(err).Err()
	case errors.As(err, &validation):
		s, _ := status.New(codes.InvalidArgument, validation.Error()).WithDetails(&errdetails.BadRequest{
			FieldViolations: []*errdetails.BadRequest_FieldViolation{{Field: validation.Field, Description: validation.Description}},
		})
		return s.Err()
	case errors.Is(err, domain.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.As(err, &dataLoss):
		return status.Error(codes.DataLoss, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}
