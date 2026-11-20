package grpc

import (
	"context"
	"errors"

	pb "outages-bot/gen/schedules/v1"
	"outages-bot/internal/schedules/application/core/domain"
	"outages-bot/internal/schedules/ports"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Adapter struct {
	pb.UnimplementedScheduleServiceServer
	api ports.APIPort
}

func NewAdapter(api ports.APIPort) *Adapter { return &Adapter{api: api} }

func (a *Adapter) FetchSchedules(ctx context.Context, _ *pb.FetchSchedulesRequest) (*pb.FetchSchedulesResponse, error) {
	schedules, err := a.api.FetchSchedules(ctx)
	if err != nil {
		return nil, toStatus(err)
	}
	response := &pb.FetchSchedulesResponse{}
	for _, s := range schedules {
		response.Schedules = append(response.Schedules, &pb.Schedule{Date: s.Date, Updated: s.Updated, Lines: s.Lines})
	}
	return response, nil
}

func toStatus(err error) error {
	code := codes.Internal
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return status.FromContextError(err).Err()
	case errors.Is(err, domain.ErrFeedUnavailable):
		code = codes.Unavailable
	case errors.Is(err, domain.ErrInvalidFeed):
		code = codes.DataLoss
	}
	return status.Error(code, err.Error())
}
