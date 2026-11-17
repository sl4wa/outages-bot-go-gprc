package grpc

import (
	"context"

	pb "outages-bot/gen/subscriptions/v1"
	"outages-bot/internal/subscriptions/application/core/domain"
	"outages-bot/internal/subscriptions/ports"
)

type Adapter struct {
	pb.UnimplementedSubscriptionServiceServer
	api ports.APIPort
}

func NewAdapter(api ports.APIPort) *Adapter { return &Adapter{api: api} }

func (a *Adapter) SearchStreets(ctx context.Context, req *pb.SearchStreetsRequest) (*pb.SearchStreetsResponse, error) {
	result, err := a.api.SearchStreets(ctx, req.GetQuery())
	if err != nil {
		return nil, toStatus(err)
	}
	response := &pb.SearchStreetsResponse{TooMany: result.TooMany}
	for _, street := range result.Streets {
		response.Streets = append(response.Streets, streetMessage(street))
	}
	return response, nil
}

func (a *Adapter) GetSubscription(ctx context.Context, req *pb.GetSubscriptionRequest) (*pb.Subscription, error) {
	result, err := a.api.GetSubscription(ctx, req.GetChatId())
	if err != nil {
		return nil, toStatus(err)
	}
	return subscriptionMessage(result), nil
}

func (a *Adapter) SetSubscription(ctx context.Context, req *pb.SetSubscriptionRequest) (*pb.Subscription, error) {
	result, err := a.api.SetSubscription(ctx, req.GetChatId(), req.GetStreetId(), req.GetBuilding())
	if err != nil {
		return nil, toStatus(err)
	}
	return subscriptionMessage(result), nil
}

func (a *Adapter) DeleteSubscription(ctx context.Context, req *pb.DeleteSubscriptionRequest) (*pb.DeleteSubscriptionResponse, error) {
	var expected *domain.SubscriptionGuard
	if req.GetExpected() != nil {
		expected = &domain.SubscriptionGuard{Generation: req.GetExpected().GetGeneration()}
	}
	deleted, err := a.api.DeleteSubscription(ctx, req.GetChatId(), expected)
	if err != nil {
		return nil, toStatus(err)
	}
	return &pb.DeleteSubscriptionResponse{Deleted: deleted}, nil
}

func (a *Adapter) ListSubscriptions(_ *pb.ListSubscriptionsRequest, stream pb.SubscriptionService_ListSubscriptionsServer) error {
	snapshot, err := a.api.ListSubscriptions(stream.Context())
	if err != nil {
		return toStatus(err)
	}
	for _, subscription := range snapshot {
		if err := stream.Send(subscriptionMessage(subscription)); err != nil {
			return err
		}
	}
	return nil
}

func (a *Adapter) MarkNotified(ctx context.Context, req *pb.MarkNotifiedRequest) (*pb.MarkNotifiedResponse, error) {
	var notification *domain.Notification
	if n := req.GetNotification(); n != nil {
		notification = &domain.Notification{StartDate: n.StartDate, EndDate: n.EndDate, Comment: n.Comment}
	}
	applied, err := a.api.MarkNotified(ctx, req.GetChatId(), domain.SubscriptionGuard{Generation: req.GetExpected().GetGeneration()}, notification)
	if err != nil {
		return nil, toStatus(err)
	}
	return &pb.MarkNotifiedResponse{Applied: applied}, nil
}

func streetMessage(street domain.Street) *pb.Street {
	return &pb.Street{Id: street.ID, Name: street.Name}
}

func subscriptionMessage(s domain.Subscription) *pb.Subscription {
	message := &pb.Subscription{
		ChatId: s.ChatID, Address: &pb.Address{Street: streetMessage(s.Address.Street), Building: s.Address.Building}, Generation: s.Generation,
	}
	if n := s.LastNotification; n != nil {
		message.LastNotification = &pb.Notification{StartDate: n.StartDate, EndDate: n.EndDate, Comment: n.Comment}
	}
	return message
}
