// Package notifier coordinates services through their protobuf contracts. It
// owns no subscriber files and commits delivery progress through gRPC.
package notifier

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	notificationsv1 "outages-bot/gen/notifications/v1"
	outagesv1 "outages-bot/gen/outages/v1"
	subscriptionsv1 "outages-bot/gen/subscriptions/v1"
	"outages-bot/internal/platform/telegram"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
)

type Sender interface {
	Send(context.Context, int64, string) error
}

type Server struct {
	notificationsv1.UnimplementedNotificationServiceServer
	subscriptions subscriptionsv1.SubscriptionServiceClient
	outages       outagesv1.OutageServiceClient
	sender        Sender
	run           sync.Mutex
}

func New(subscriptions subscriptionsv1.SubscriptionServiceClient, outages outagesv1.OutageServiceClient, sender Sender) *Server {
	return &Server{subscriptions: subscriptions, outages: outages, sender: sender}
}

// Run enables the background loop. An interval of zero leaves RunOnce available
// without performing any automatic checks. Its lifetime belongs to the caller.
func (s *Server) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		if _, err := s.RunOnce(ctx, &notificationsv1.RunOnceRequest{}); err != nil && ctx.Err() == nil && status.Code(err) != codes.FailedPrecondition {
			slog.ErrorContext(ctx, "automatic notification run failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Server) RunOnce(ctx context.Context, _ *notificationsv1.RunOnceRequest) (*notificationsv1.RunOnceResponse, error) {
	if !s.run.TryLock() {
		return nil, status.Error(codes.FailedPrecondition, "a notification run is already in progress")
	}
	defer s.run.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	result := &notificationsv1.RunOnceResponse{}
	defer func() {
		slog.InfoContext(ctx, "notification run finished", "subscribers", result.Subscribers, "matched", result.Matched,
			"sent", result.Sent, "skipped", result.Skipped, "unsubscribed", result.Unsubscribed)
	}()

	fetchCtx, fetchCancel := context.WithTimeout(ctx, 25*time.Second)
	snapshot, err := s.outages.FetchSnapshot(fetchCtx, &outagesv1.FetchSnapshotRequest{})
	fetchCancel()
	if err != nil {
		return nil, dependencyError("fetch outage snapshot", err)
	}
	// Finish the stream before sending anything, so a truncated subscription
	// snapshot cannot cause a partly evaluated batch.
	subscribers, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	result.Subscribers = int32(len(subscribers))
	for _, subscriber := range subscribers {
		if ctx.Err() != nil {
			return nil, status.FromContextError(ctx.Err()).Err()
		}
		outage := matchingOutage(subscriber, snapshot.Outages)
		if outage == nil {
			result.Skipped++
			continue
		}
		result.Matched++
		sendCtx, sendCancel := context.WithTimeout(ctx, 20*time.Second)
		err := s.sender.Send(sendCtx, subscriber.ChatId, notificationText(outage))
		sendCancel()
		if err != nil {
			var telegramErr *telegram.Error
			if errors.As(err, &telegramErr) {
				switch telegramErr.StatusCode {
				case 400:
					result.Skipped++
					continue
				case 403:
					deleteCtx, deleteCancel := context.WithTimeout(ctx, 5*time.Second)
					deleted, deleteErr := s.subscriptions.DeleteSubscription(deleteCtx, &subscriptionsv1.DeleteSubscriptionRequest{
						ChatId: subscriber.ChatId, Expected: guard(subscriber),
					})
					deleteCancel()
					if deleteErr != nil {
						return nil, dependencyError("unsubscribe blocked recipient", deleteErr)
					}
					if deleted.Deleted {
						result.Unsubscribed++
					} else {
						result.Skipped++
					}
					continue
				case 429:
					return nil, rateLimitError(telegramErr.RetryAfter)
				}
			}
			if ctx.Err() != nil {
				return nil, status.FromContextError(ctx.Err()).Err()
			}
			return nil, status.Error(codes.Unavailable, "Telegram delivery failed; prior deliveries remain acknowledged")
		}
		result.Sent++
		// Telegram accepted the message. Even if the original caller has just
		// canceled, make one bounded attempt to save that fact. A crash between
		// send and acknowledgment can still cause duplicate delivery.
		ackCtx, ackCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		_, err = s.subscriptions.MarkNotified(ackCtx, &subscriptionsv1.MarkNotifiedRequest{
			ChatId: subscriber.ChatId, Expected: guard(subscriber),
			Notification: &subscriptionsv1.Notification{StartDate: outage.StartDate, EndDate: outage.EndDate, Comment: outage.Comment},
		})
		ackCancel()
		if err != nil {
			return nil, dependencyError("acknowledge delivered notification", err)
		}
	}
	return result, nil
}

func (s *Server) list(ctx context.Context) ([]*subscriptionsv1.Subscription, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	stream, err := s.subscriptions.ListSubscriptions(ctx, &subscriptionsv1.ListSubscriptionsRequest{})
	if err != nil {
		return nil, dependencyError("list subscriptions", err)
	}
	var result []*subscriptionsv1.Subscription
	for {
		subscriber, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return result, nil
		}
		if err != nil {
			return nil, dependencyError("receive subscriptions", err)
		}
		result = append(result, subscriber)
	}
}

func dependencyError(operation string, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		err = status.FromContextError(err).Err()
	}
	// Status.Proto returns a copy, retaining rich protobuf error details while
	// adding the operation that failed at this service boundary.
	s := status.Convert(err).Proto()
	if codes.Code(s.Code) == codes.Unknown {
		s.Code = int32(codes.Unavailable)
	}
	s.Message = operation + ": " + s.Message
	return status.FromProto(s).Err()
}

func rateLimitError(retryAfter int) error {
	s := status.New(codes.ResourceExhausted, "Telegram rate limit; retry on a later run")
	if retryAfter > 0 {
		// Use seconds directly: a remote integer must not overflow time.Duration.
		delay := &durationpb.Duration{Seconds: int64(retryAfter)}
		if delay.CheckValid() == nil {
			withDetails, err := s.WithDetails(&errdetails.RetryInfo{RetryDelay: delay})
			if err == nil {
				s = withDetails
			}
		}
	}
	return s.Err()
}

func guard(s *subscriptionsv1.Subscription) *subscriptionsv1.SubscriptionGuard {
	return &subscriptionsv1.SubscriptionGuard{Generation: s.Generation}
}

// Send at most one outage per address. If any current match is already recorded
// as delivered, skip the address even when other matching outages are new.
func matchingOutage(s *subscriptionsv1.Subscription, outages []*outagesv1.Outage) *outagesv1.Outage {
	var first *outagesv1.Outage
	for _, outage := range outages {
		if outage.StreetId != s.GetAddress().GetStreet().GetId() {
			continue
		}
		matches := false
		for _, building := range outage.Buildings {
			if building == s.GetAddress().GetBuilding() {
				matches = true
				break
			}
		}
		if !matches {
			continue
		}
		last := s.LastNotification
		if last != nil && last.StartDate == outage.StartDate && last.EndDate == outage.EndDate && last.Comment == outage.Comment {
			return nil
		}
		if first == nil {
			first = outage
		}
	}
	return first
}

func notificationText(o *outagesv1.Outage) string {
	return fmt.Sprintf("Поточні відключення:\nМісто: %s\nВулиця: %s\n<b>%s – %s</b>\nКоментар: %s\nБудинки: %s",
		html.EscapeString(o.City), html.EscapeString(o.StreetName), html.EscapeString(displayTime(o.StartDate)),
		html.EscapeString(displayTime(o.EndDate)), html.EscapeString(o.Comment), html.EscapeString(strings.Join(o.Buildings, ", ")))
}

func displayTime(value string) string {
	// Preserve the upstream local wall time rather than applying a timezone
	// conversion. The source feed normally supplies ISO 8601 timestamps.
	if day, clock, ok := strings.Cut(value, "T"); ok && len(clock) >= 5 {
		return day + " " + clock[:5]
	}
	return value
}
