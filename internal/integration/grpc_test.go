package integration_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	nv1 "outages-bot/gen/notifications/v1"
	ov1 "outages-bot/gen/outages/v1"
	scv1 "outages-bot/gen/schedules/v1"
	sv1 "outages-bot/gen/subscriptions/v1"
	notifiergrpc "outages-bot/internal/notifier/adapters/grpc"
	notifieroutages "outages-bot/internal/notifier/adapters/outages"
	notifierschedules "outages-bot/internal/notifier/adapters/schedules"
	notifiersubscriptions "outages-bot/internal/notifier/adapters/subscriptions"
	notifiertelegram "outages-bot/internal/notifier/adapters/telegram"
	notifierapi "outages-bot/internal/notifier/application/core/api"
	outagegrpc "outages-bot/internal/outages/adapters/grpc"
	"outages-bot/internal/outages/adapters/loe"
	outageapi "outages-bot/internal/outages/application/core/api"
	"outages-bot/internal/platform/rpc"
	"outages-bot/internal/platform/telegram"
	schedulegrpc "outages-bot/internal/schedules/adapters/grpc"
	scheduleloe "outages-bot/internal/schedules/adapters/loe"
	scheduleapi "outages-bot/internal/schedules/application/core/api"
	"outages-bot/internal/subscriptions/adapters/catalog"
	"outages-bot/internal/subscriptions/adapters/db"
	subscriptiongrpc "outages-bot/internal/subscriptions/adapters/grpc"
	subscriptionapi "outages-bot/internal/subscriptions/application/core/api"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func serve(t *testing.T, register func(*grpc.Server), opts ...grpc.DialOption) (*grpc.ClientConn, func()) {
	t.Helper()
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	register(server)
	go func() { _ = server.Serve(listener) }()
	opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }))
	conn, err := grpc.NewClient("passthrough:///test", opts...)
	if err != nil {
		t.Fatal(err)
	}
	stop := func() { _ = conn.Close(); server.Stop(); _ = listener.Close() }
	t.Cleanup(stop)
	return conn, stop
}

func subscriptionClient(t *testing.T, dir string) (sv1.SubscriptionServiceClient, func()) {
	t.Helper()
	repository, err := db.NewAdapter(dir)
	if err != nil {
		t.Fatal(err)
	}
	streets, err := catalog.Load()
	if err != nil {
		t.Fatal(err)
	}
	application := subscriptionapi.NewApplication(repository, streets)
	conn, stop := serve(t, func(s *grpc.Server) {
		sv1.RegisterSubscriptionServiceServer(s, subscriptiongrpc.NewAdapter(application))
	})
	return sv1.NewSubscriptionServiceClient(conn), stop
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }

type sendFunc func(context.Context, int64, string) error

func (fn sendFunc) Send(ctx context.Context, id int64, text string) error { return fn(ctx, id, text) }

func outageClient(t *testing.T, transport roundTripFunc, opts ...grpc.DialOption) ov1.OutageServiceClient {
	t.Helper()
	feedAdapter := loe.NewAdapter("https://outages.example.invalid/", &http.Client{Transport: transport})
	application := outageapi.NewApplication(feedAdapter)
	conn, _ := serve(t, func(s *grpc.Server) {
		ov1.RegisterOutageServiceServer(s, outagegrpc.NewAdapter(application))
	}, opts...)
	return ov1.NewOutageServiceClient(conn)
}

func notificationAdapter(subscriptions sv1.SubscriptionServiceClient, outages ov1.OutageServiceClient, sender notifiertelegram.Client) *notifiergrpc.Adapter {
	subscriptionAdapter := notifiersubscriptions.NewAdapter(subscriptions)
	outageAdapter := notifieroutages.NewAdapter(outages)
	senderAdapter := notifiertelegram.NewAdapter(sender)
	application := notifierapi.NewApplication(subscriptionAdapter, outageAdapter, nil, senderAdapter, time.Local)
	return notifiergrpc.NewAdapter(application)
}

func TestSubscriptionsOverGRPC(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	dir := t.TempDir()
	client, stop := subscriptionClient(t, dir)
	first, err := client.SetSubscription(ctx, &sv1.SetSubscriptionRequest{ChatId: -42, StreetId: 6458, Building: "13a"})
	if err != nil || first.GetAddress().GetBuilding() != "13a" {
		t.Fatalf("subscription must preserve building input: %v %v", first, err)
	}
	got, err := client.GetSubscription(ctx, &sv1.GetSubscriptionRequest{ChatId: -42})
	if err != nil || got.Generation != first.Generation {
		t.Fatalf("get subscription: %v %v", got, err)
	}
	stream, err := client.ListSubscriptions(ctx, &sv1.ListSubscriptionsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	listed, err := stream.Recv()
	if err != nil || listed.GetChatId() != -42 {
		t.Fatalf("stream subscription: %v %v", listed, err)
	}
	if _, err := stream.Recv(); err != io.EOF {
		t.Fatalf("finite stream must end: %v", err)
	}
	_, err = client.SetSubscription(ctx, &sv1.SetSubscriptionRequest{ChatId: -42, StreetId: 6458, Building: " "})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected INVALID_ARGUMENT: %v", err)
	}
	details := status.Convert(err).Details()
	if len(details) != 1 {
		t.Fatalf("missing validation detail: %v", details)
	}
	badRequest, ok := details[0].(*errdetails.BadRequest)
	if !ok || len(badRequest.FieldViolations) != 1 || badRequest.FieldViolations[0].Field != "building" {
		t.Fatalf("unexpected validation detail: %v", details)
	}
	replacement, err := client.SetSubscription(ctx, &sv1.SetSubscriptionRequest{ChatId: -42, StreetId: 6458, Building: "13-А"})
	if err != nil {
		t.Fatal(err)
	}
	marked, err := client.MarkNotified(ctx, &sv1.MarkNotifiedRequest{
		ChatId: -42, Expected: &sv1.SubscriptionGuard{Generation: first.Generation},
		Notification: &sv1.Notification{StartDate: "start", EndDate: "end"},
	})
	if err != nil || marked.GetApplied() {
		t.Fatalf("stale acknowledgment must not change replacement: %v %v", marked, err)
	}
	deleted, err := client.DeleteSubscription(ctx, &sv1.DeleteSubscriptionRequest{
		ChatId: -42, Expected: &sv1.SubscriptionGuard{Generation: first.Generation},
	})
	if err != nil || deleted.GetDeleted() {
		t.Fatalf("stale delete must retain replacement: %v %v", deleted, err)
	}
	_, err = client.MarkNotified(ctx, &sv1.MarkNotifiedRequest{
		ChatId: -42, Expected: &sv1.SubscriptionGuard{}, Notification: &sv1.Notification{StartDate: "start", EndDate: "end"},
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("empty generation must be rejected: %v", err)
	}
	marked, err = client.MarkNotified(ctx, &sv1.MarkNotifiedRequest{
		ChatId: -42, Expected: &sv1.SubscriptionGuard{Generation: replacement.Generation},
		Notification: &sv1.Notification{StartDate: "start", EndDate: "end", Comment: "delivered"},
	})
	if err != nil || !marked.GetApplied() {
		t.Fatalf("current generation must acknowledge: %v %v", marked, err)
	}

	stop()
	restarted, _ := subscriptionClient(t, dir)
	got, err = restarted.GetSubscription(ctx, &sv1.GetSubscriptionRequest{ChatId: -42})
	if err != nil || got.Generation != replacement.Generation || got.GetLastNotification().GetComment() != "delivered" {
		t.Fatalf("JSON restart must retain replacement: %v %v", got, err)
	}
}

func TestNotificationProgressOverGRPC(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	subs, _ := subscriptionClient(t, t.TempDir())
	for _, id := range []int64{1, 2} {
		if _, err := subs.SetSubscription(ctx, &sv1.SetSubscriptionRequest{ChatId: id, StreetId: 6458, Building: "13-А"}); err != nil {
			t.Fatal(err)
		}
	}
	// Change the feed order after partial delivery. A delivered matching row
	// must suppress the address even when another matching row appears first.
	rowA := `{"dateEvent":"2026-11-13T09:00:00+02:00","datePlanIn":"2026-11-13T12:00:00+02:00","city":{"name":"Львів"},"street":{"id":6458,"name":"Хмельницького Б."},"buildingNames":"13-А","koment":"Repair <wire> A"}`
	rowB := strings.ReplaceAll(rowA, "09:00", "15:00")
	rowB = strings.ReplaceAll(rowB, "12:00", "17:00")
	rowB = strings.ReplaceAll(rowB, "<wire> A", "<wire> B")
	var body atomic.Value
	body.Store("[" + rowA + "," + rowB + "]")
	feed := outageClient(t, func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body.Load().(string)))}, nil
	})
	var firstAttempts, secondAttempts atomic.Int32
	sender := sendFunc(func(_ context.Context, id int64, text string) error {
		if !strings.Contains(text, "Repair &lt;wire&gt;") {
			t.Error("notification must escape upstream HTML")
		}
		if id == 1 {
			firstAttempts.Add(1)
		} else if secondAttempts.Add(1) == 1 {
			return errors.New("temporary delivery failure")
		}
		return nil
	})
	conn, _ := serve(t, func(s *grpc.Server) {
		nv1.RegisterNotificationServiceServer(s, notificationAdapter(subs, feed, sender))
	})
	client := nv1.NewNotificationServiceClient(conn)
	if _, err := client.RunOnce(ctx, &nv1.RunOnceRequest{}); status.Code(err) != codes.Unavailable {
		t.Fatalf("expected partial delivery failure: %v", err)
	}
	first, err := subs.GetSubscription(ctx, &sv1.GetSubscriptionRequest{ChatId: 1})
	if err != nil || first.GetLastNotification().GetStartDate() == "" {
		t.Fatalf("successful delivery must remain acknowledged: %v %v", first, err)
	}
	body.Store(`{"hydra:member":[` + rowB + "," + rowA + "]}")
	retried, err := client.RunOnce(ctx, &nv1.RunOnceRequest{})
	if err != nil || retried.GetSent() != 1 {
		t.Fatalf("next run must deliver only the failed recipient: %v %v", retried, err)
	}
	repeated, err := client.RunOnce(ctx, &nv1.RunOnceRequest{})
	if err != nil || repeated.GetSent() != 0 || firstAttempts.Load() != 1 || secondAttempts.Load() != 2 {
		t.Fatalf("unchanged outages must not resend: %v %v", repeated, err)
	}
}

func TestNotifierRetriesUnavailableFeed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	subs, _ := subscriptionClient(t, t.TempDir())
	if _, err := subs.SetSubscription(ctx, &sv1.SetSubscriptionRequest{ChatId: 1, StreetId: 6458, Building: "13-А"}); err != nil {
		t.Fatal(err)
	}
	row := `[{"dateEvent":"2026-11-13T09:00","datePlanIn":"2026-11-13T12:00","city":{"name":"Львів"},"street":{"id":6458,"name":"Хмельницького Б."},"buildingNames":"13-А"}]`
	var requests atomic.Int32
	// LOE fails once, outages returns UNAVAILABLE, and the notifier's retry succeeds.
	feed := outageClient(t, func(*http.Request) (*http.Response, error) {
		if requests.Add(1) == 1 {
			return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(strings.NewReader(""))}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(row))}, nil
	}, rpc.Retry()...)
	notifier := notificationAdapter(subs, feed, sendFunc(func(context.Context, int64, string) error { return nil }))
	result, err := notifier.RunOnce(ctx, &nv1.RunOnceRequest{})
	if err != nil || result.GetSent() != 1 || requests.Load() != 2 {
		t.Fatalf("retry must recover from one UNAVAILABLE: %v %v, %d requests", result, err, requests.Load())
	}
}

func TestCancellationReachesUpstream(t *testing.T) {
	started, canceled := make(chan struct{}), make(chan struct{})
	feed := outageClient(t, func(req *http.Request) (*http.Response, error) {
		close(started)
		<-req.Context().Done()
		close(canceled)
		return nil, req.Context().Err()
	})
	subs, _ := subscriptionClient(t, t.TempDir())
	conn, _ := serve(t, func(s *grpc.Server) {
		nv1.RegisterNotificationServiceServer(s, notificationAdapter(subs, feed, sendFunc(func(context.Context, int64, string) error {
			t.Error("canceled fetch must not send a notification")
			return nil
		})))
	})
	client := nv1.NewNotificationServiceClient(conn)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() { _, err := client.RunOnce(ctx, &nv1.RunOnceRequest{}); finished <- err }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("upstream request did not start")
	}
	cancel()
	select {
	case err := <-finished:
		if status.Code(err) != codes.Canceled {
			t.Fatalf("expected CANCELED: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("notification RPC did not stop")
	}
	select {
	case <-canceled:
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation did not cross the downstream RPC and HTTP request")
	}
}

type scheduleSender struct{ sent map[int64]string }

func (s scheduleSender) Send(_ context.Context, id int64, text string) error {
	if id == 2 {
		return &telegram.Error{StatusCode: http.StatusForbidden}
	}
	s.sent[id] = text
	return nil
}

func TestScheduleBroadcastOverGRPC(t *testing.T) {
	day := time.Now().Format("02.01.2006")
	body := `{"hydra:member":[{"menuItems":[{"id":1,"rawHtml":"<p>Графік погодинних відключень на ` + day + `</p><p>Інформація станом на 08:48 ` + day + `</p><p>Група 1.1. 10:00-12:00</p>"}]}]}`
	feed := scheduleloe.NewAdapter("https://schedules.example.invalid/", &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})})
	conn, _ := serve(t, func(s *grpc.Server) {
		scv1.RegisterScheduleServiceServer(s, schedulegrpc.NewAdapter(scheduleapi.NewApplication(feed)))
	})
	subs, _ := subscriptionClient(t, t.TempDir())
	ctx := context.Background()
	for _, chat := range []int64{1, 2} {
		if _, err := subs.SetSubscription(ctx, &sv1.SetSubscriptionRequest{ChatId: chat, StreetId: 6458, Building: "13a"}); err != nil {
			t.Fatal(err)
		}
	}
	sender := scheduleSender{sent: map[int64]string{}}
	application := notifierapi.NewApplication(notifiersubscriptions.NewAdapter(subs), nil,
		notifierschedules.NewAdapter(scv1.NewScheduleServiceClient(conn)), notifiertelegram.NewAdapter(sender), time.Local)
	if err := application.CheckSchedules(ctx); err != nil || len(sender.sent) != 0 {
		t.Fatalf("first check must only record schedules: %v %v", sender.sent, err)
	}
	body = strings.Replace(body, "10:00-12:00", "11:00-13:00", 1)
	if err := application.CheckSchedules(ctx); err != nil {
		t.Fatal(err)
	}
	if got := sender.sent[1]; !strings.Contains(got, "<b>Графік відключень на сьогодні</b>") || !strings.Contains(got, "11:00-13:00") {
		t.Fatalf("subscriber did not receive the changed schedule: %q", got)
	}
	if _, err := subs.GetSubscription(ctx, &sv1.GetSubscriptionRequest{ChatId: 2}); status.Code(err) != codes.NotFound {
		t.Fatalf("blocked subscriber must be removed: %v", err)
	}
	if _, err := subs.GetSubscription(ctx, &sv1.GetSubscriptionRequest{ChatId: 1}); err != nil {
		t.Fatalf("other subscriber must remain: %v", err)
	}
}
