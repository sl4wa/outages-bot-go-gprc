package subscriptions

import (
	"context"
	"errors"
	"io"

	pb "outages-bot/gen/subscriptions/v1"
	"outages-bot/internal/notifier/application/core/domain"
	"outages-bot/internal/notifier/ports"
	"outages-bot/internal/platform/rpc"
)

type Adapter struct {
	client pb.SubscriptionServiceClient
}

var _ ports.SubscriptionPort = (*Adapter)(nil)

func NewAdapter(client pb.SubscriptionServiceClient) *Adapter { return &Adapter{client: client} }

func (a *Adapter) List(ctx context.Context) ([]domain.Subscription, error) {
	stream, err := a.client.ListSubscriptions(ctx, &pb.ListSubscriptionsRequest{})
	if err != nil {
		return nil, rpc.DependencyError("list subscriptions", err)
	}
	var result []domain.Subscription
	for {
		s, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return result, nil
		}
		if err != nil {
			return nil, rpc.DependencyError("receive subscriptions", err)
		}
		subscription := domain.Subscription{
			ChatID: s.GetChatId(), StreetID: s.GetAddress().GetStreet().GetId(),
			Building: s.GetAddress().GetBuilding(), Generation: s.GetGeneration(),
		}
		if n := s.GetLastNotification(); n != nil {
			subscription.LastNotification = &domain.Notification{StartDate: n.StartDate, EndDate: n.EndDate, Comment: n.Comment}
		}
		result = append(result, subscription)
	}
}

func (a *Adapter) Delete(ctx context.Context, subscription domain.Subscription) (bool, error) {
	result, err := a.client.DeleteSubscription(ctx, &pb.DeleteSubscriptionRequest{
		ChatId: subscription.ChatID, Expected: &pb.SubscriptionGuard{Generation: subscription.Generation},
	})
	if err != nil {
		return false, rpc.DependencyError("unsubscribe blocked recipient", err)
	}
	return result.Deleted, nil
}

func (a *Adapter) MarkNotified(ctx context.Context, subscription domain.Subscription, notification domain.Notification) error {
	_, err := a.client.MarkNotified(ctx, &pb.MarkNotifiedRequest{
		ChatId: subscription.ChatID, Expected: &pb.SubscriptionGuard{Generation: subscription.Generation},
		Notification: &pb.Notification{StartDate: notification.StartDate, EndDate: notification.EndDate, Comment: notification.Comment},
	})
	if err != nil {
		return rpc.DependencyError("acknowledge delivered notification", err)
	}
	return nil
}
