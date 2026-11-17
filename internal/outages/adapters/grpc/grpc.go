package grpc

import (
	"context"
	"errors"

	pb "outages-bot/gen/outages/v1"
	"outages-bot/internal/outages/application/core/domain"
	"outages-bot/internal/outages/ports"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Adapter struct {
	pb.UnimplementedOutageServiceServer
	api ports.APIPort
}

func NewAdapter(api ports.APIPort) *Adapter { return &Adapter{api: api} }

func (a *Adapter) FetchSnapshot(ctx context.Context, _ *pb.FetchSnapshotRequest) (*pb.FetchSnapshotResponse, error) {
	rows, err := a.api.FetchSnapshot(ctx)
	if err != nil {
		return nil, toStatus(err)
	}
	response := &pb.FetchSnapshotResponse{}
	for _, row := range rows {
		response.Outages = append(response.Outages, &pb.Outage{
			StartDate: row.StartDate, EndDate: row.EndDate, City: row.City,
			StreetId: row.StreetID, StreetName: row.StreetName, Buildings: row.Buildings, Comment: row.Comment,
		})
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
	case errors.Is(err, domain.ErrInvalidSnapshot):
		code = codes.DataLoss
	case errors.Is(err, domain.ErrSnapshotTooLarge):
		code = codes.ResourceExhausted
	}
	return status.Error(code, err.Error())
}
