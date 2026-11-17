package subscriptions

import (
	"context"

	pb "outages-bot/gen/subscriptions/v1"
	"outages-bot/internal/bot/application/core/domain"
	"outages-bot/internal/bot/ports"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Adapter struct {
	client pb.SubscriptionServiceClient
}

var _ ports.SubscriptionPort = (*Adapter)(nil)

func NewAdapter(client pb.SubscriptionServiceClient) *Adapter { return &Adapter{client: client} }

func (a *Adapter) Get(ctx context.Context, chatID int64) (domain.Subscription, error) {
	result, err := a.client.GetSubscription(ctx, &pb.GetSubscriptionRequest{ChatId: chatID})
	if status.Code(err) == codes.NotFound {
		return domain.Subscription{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Subscription{}, err
	}
	return subscription(result), nil
}

func (a *Adapter) Set(ctx context.Context, chatID int64, streetID int32, building string) (domain.Subscription, error) {
	result, err := a.client.SetSubscription(ctx, &pb.SetSubscriptionRequest{ChatId: chatID, StreetId: streetID, Building: building})
	if err != nil {
		return domain.Subscription{}, err
	}
	return subscription(result), nil
}

func (a *Adapter) Delete(ctx context.Context, chatID int64) (bool, error) {
	result, err := a.client.DeleteSubscription(ctx, &pb.DeleteSubscriptionRequest{ChatId: chatID})
	if err != nil {
		return false, err
	}
	return result.Deleted, nil
}

func (a *Adapter) SearchStreets(ctx context.Context, query string) (domain.StreetSearch, error) {
	result, err := a.client.SearchStreets(ctx, &pb.SearchStreetsRequest{Query: query})
	if err != nil {
		return domain.StreetSearch{}, err
	}
	search := domain.StreetSearch{TooMany: result.TooMany}
	for _, street := range result.Streets {
		search.Streets = append(search.Streets, domain.Street{ID: street.Id, Name: street.Name})
	}
	return search, nil
}

func subscription(s *pb.Subscription) domain.Subscription {
	return domain.Subscription{Address: domain.Address{
		Street:   domain.Street{ID: s.GetAddress().GetStreet().GetId(), Name: s.GetAddress().GetStreet().GetName()},
		Building: s.GetAddress().GetBuilding(),
	}}
}
