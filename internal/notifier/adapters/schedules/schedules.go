package schedules

import (
	"context"

	pb "outages-bot/gen/schedules/v1"
	"outages-bot/internal/notifier/application/core/domain"
	"outages-bot/internal/notifier/ports"
	"outages-bot/internal/platform/rpc"
)

type Adapter struct {
	client pb.ScheduleServiceClient
}

var _ ports.ScheduleFeedPort = (*Adapter)(nil)

func NewAdapter(client pb.ScheduleServiceClient) *Adapter { return &Adapter{client: client} }

func (a *Adapter) FetchSchedules(ctx context.Context) ([]domain.Schedule, error) {
	response, err := a.client.FetchSchedules(ctx, &pb.FetchSchedulesRequest{})
	if err != nil {
		return nil, rpc.DependencyError("fetch schedules", err)
	}
	result := make([]domain.Schedule, 0, len(response.Schedules))
	for _, s := range response.Schedules {
		result = append(result, domain.Schedule{Date: s.Date, Updated: s.Updated, Lines: s.Lines})
	}
	return result, nil
}
