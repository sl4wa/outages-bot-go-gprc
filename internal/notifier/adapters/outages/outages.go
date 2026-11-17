package outages

import (
	"context"

	pb "outages-bot/gen/outages/v1"
	"outages-bot/internal/notifier/application/core/domain"
	"outages-bot/internal/notifier/ports"
	"outages-bot/internal/platform/rpc"
)

type Adapter struct {
	client pb.OutageServiceClient
}

var _ ports.OutagePort = (*Adapter)(nil)

func NewAdapter(client pb.OutageServiceClient) *Adapter { return &Adapter{client: client} }

func (a *Adapter) FetchSnapshot(ctx context.Context) ([]domain.Outage, error) {
	snapshot, err := a.client.FetchSnapshot(ctx, &pb.FetchSnapshotRequest{})
	if err != nil {
		return nil, rpc.DependencyError("fetch outage snapshot", err)
	}
	result := make([]domain.Outage, 0, len(snapshot.Outages))
	for _, row := range snapshot.Outages {
		result = append(result, domain.Outage{
			StartDate: row.StartDate, EndDate: row.EndDate, City: row.City,
			StreetID: row.StreetId, StreetName: row.StreetName, Buildings: row.Buildings, Comment: row.Comment,
		})
	}
	return result, nil
}
