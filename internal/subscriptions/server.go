// Package subscriptions owns subscriber records and the street catalog.
// No other process should access usersDir while this service is running.
package subscriptions

import (
	"bufio"
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	pb "outages-bot/gen/subscriptions/v1"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

//go:embed streets.tsv
var streetData string

// Server is the single owner of subscription files. Its mutex protects the
// complete read/check/write operation, not merely individual file writes.
type Server struct {
	pb.UnimplementedSubscriptionServiceServer
	mu       sync.RWMutex
	usersDir string
	streets  []*pb.Street
	byID     map[int32]*pb.Street
	records  map[int64]record
}

type record struct {
	StreetID   int32  `json:"street_id"`
	StreetName string `json:"street_name"`
	Building   string `json:"building"`
	Generation string `json:"generation"`
	StartDate  string `json:"start_date,omitempty"`
	EndDate    string `json:"end_date,omitempty"`
	Comment    string `json:"comment,omitempty"`
}

func New(usersDir string) (*Server, error) {
	if usersDir == "" {
		return nil, fmt.Errorf("users directory is required")
	}
	if err := os.MkdirAll(usersDir, 0700); err != nil {
		return nil, fmt.Errorf("create users directory: %w", err)
	}
	s := &Server{usersDir: usersDir, byID: make(map[int32]*pb.Street), records: make(map[int64]record)}
	scanner := bufio.NewScanner(strings.NewReader(streetData))
	for scanner.Scan() {
		fields := strings.SplitN(scanner.Text(), "\t", 2)
		if len(fields) != 2 {
			return nil, fmt.Errorf("invalid bundled street row")
		}
		id, err := strconv.ParseInt(fields[0], 10, 32)
		if err != nil || id <= 0 || strings.TrimSpace(fields[1]) == "" {
			return nil, fmt.Errorf("invalid bundled street")
		}
		street := &pb.Street{Id: int32(id), Name: fields[1]}
		if s.byID[street.Id] != nil {
			return nil, fmt.Errorf("duplicate bundled street ID %d", id)
		}
		s.streets = append(s.streets, street)
		s.byID[street.Id] = street
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read bundled streets: %w", err)
	}
	if err := s.loadRecords(); err != nil {
		return nil, fmt.Errorf("load subscriptions: %w", err)
	}
	return s, nil
}

func (s *Server) SearchStreets(ctx context.Context, req *pb.SearchStreetsRequest) (*pb.SearchStreetsResponse, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	query := strings.ToLower(strings.TrimSpace(req.GetQuery()))
	if query == "" {
		return nil, invalidField("query", "street query is required")
	}
	result := &pb.SearchStreetsResponse{}
	for _, street := range s.streets {
		name := strings.ToLower(street.Name)
		if name == query {
			return &pb.SearchStreetsResponse{Streets: []*pb.Street{copyStreet(street)}}, nil
		}
		if strings.Contains(name, query) {
			if len(result.Streets) < 30 {
				result.Streets = append(result.Streets, copyStreet(street))
			} else {
				result.TooMany = true
			}
		}
	}
	if result.TooMany {
		result.Streets = nil
	}
	return result, nil
}

func (s *Server) GetSubscription(ctx context.Context, req *pb.GetSubscriptionRequest) (*pb.Subscription, error) {
	if err := validChat(req.GetChatId()); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	r, exists := s.records[req.ChatId]
	if !exists {
		return nil, status.Error(codes.NotFound, "subscription not found")
	}
	return r.subscription(req.ChatId), nil
}

func (s *Server) SetSubscription(ctx context.Context, req *pb.SetSubscriptionRequest) (*pb.Subscription, error) {
	if err := validChat(req.GetChatId()); err != nil {
		return nil, err
	}
	street := s.byID[req.GetStreetId()]
	if street == nil {
		return nil, invalidField("street_id", "unknown street ID")
	}
	if strings.TrimSpace(req.GetBuilding()) == "" {
		return nil, invalidField("building", "building is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return nil, status.Error(codes.Internal, "cannot generate subscription identity")
	}
	r := &record{StreetID: street.Id, StreetName: street.Name, Building: req.Building, Generation: hex.EncodeToString(token[:])}
	if err := s.save(req.ChatId, r); err != nil {
		return nil, err
	}
	s.records[req.ChatId] = *r
	return r.subscription(req.ChatId), nil
}

func (s *Server) DeleteSubscription(ctx context.Context, req *pb.DeleteSubscriptionRequest) (*pb.DeleteSubscriptionResponse, error) {
	if err := validChat(req.GetChatId()); err != nil {
		return nil, err
	}
	if req.GetExpected() != nil {
		if err := validGuard(req.Expected); err != nil {
			return nil, err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	r, exists := s.records[req.ChatId]
	if !exists {
		return &pb.DeleteSubscriptionResponse{}, nil
	}
	if req.Expected != nil && r.Generation != req.Expected.Generation {
		return &pb.DeleteSubscriptionResponse{}, nil
	}
	err := os.Remove(s.path(req.ChatId))
	if err != nil && !os.IsNotExist(err) {
		return nil, status.Error(codes.Internal, "cannot delete subscription")
	}
	delete(s.records, req.ChatId)
	return &pb.DeleteSubscriptionResponse{Deleted: true}, nil
}

func (s *Server) ListSubscriptions(_ *pb.ListSubscriptionsRequest, stream pb.SubscriptionService_ListSubscriptionsServer) error {
	snapshot, err := s.snapshot(stream.Context())
	if err != nil {
		return err
	}
	for _, sub := range snapshot {
		if err := stream.Send(sub); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) MarkNotified(ctx context.Context, req *pb.MarkNotifiedRequest) (*pb.MarkNotifiedResponse, error) {
	if err := validChat(req.GetChatId()); err != nil {
		return nil, err
	}
	if err := validGuard(req.GetExpected()); err != nil {
		return nil, err
	}
	if req.GetNotification() == nil {
		return nil, invalidField("notification", "notification is required")
	}
	if strings.TrimSpace(req.Notification.StartDate) == "" {
		return nil, invalidField("notification.start_date", "start date is required")
	}
	if strings.TrimSpace(req.Notification.EndDate) == "" {
		return nil, invalidField("notification.end_date", "end date is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	r, exists := s.records[req.ChatId]
	if !exists {
		return &pb.MarkNotifiedResponse{}, nil
	}
	if r.Generation != req.Expected.Generation {
		return &pb.MarkNotifiedResponse{}, nil
	}
	r.StartDate, r.EndDate, r.Comment = req.Notification.StartDate, req.Notification.EndDate, req.Notification.Comment
	if err := s.save(req.ChatId, &r); err != nil {
		return nil, err
	}
	s.records[req.ChatId] = r
	return &pb.MarkNotifiedResponse{Applied: true}, nil
}

func (s *Server) snapshot(ctx context.Context) ([]*pb.Subscription, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	subscriptions := make([]*pb.Subscription, 0, len(s.records))
	for id, r := range s.records {
		if err := contextError(ctx); err != nil {
			s.mu.RUnlock()
			return nil, err
		}
		subscriptions = append(subscriptions, r.subscription(id))
	}
	s.mu.RUnlock()
	sort.Slice(subscriptions, func(i, j int) bool { return subscriptions[i].ChatId < subscriptions[j].ChatId })
	return subscriptions, nil
}

func (s *Server) path(id int64) string {
	return filepath.Join(s.usersDir, strconv.FormatInt(id, 10)+".json")
}

// Load durable records once. During operation, this process owns the directory
// and the in-memory map is authoritative; external file edits require restart.
func (s *Server) loadRecords() error {
	entries, err := os.ReadDir(s.usersDir)
	if err != nil {
		return status.Error(codes.Internal, "cannot read subscriptions directory")
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		stem := strings.TrimSuffix(entry.Name(), ".json")
		id, err := strconv.ParseInt(stem, 10, 64)
		if err != nil || id == 0 || strconv.FormatInt(id, 10) != stem {
			continue
		}
		r, err := s.loadFile(id)
		if err != nil {
			return err
		}
		s.records[id] = *r
	}
	return nil
}

func (s *Server) loadFile(id int64) (*record, error) {
	data, err := os.ReadFile(s.path(id))
	if os.IsNotExist(err) {
		return nil, status.Error(codes.NotFound, "subscription not found")
	}
	if err != nil {
		return nil, status.Error(codes.Internal, "cannot read subscription")
	}
	var r record
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, status.Errorf(codes.DataLoss, "invalid JSON for chat %d", id)
	}
	if r.StreetID <= 0 || strings.TrimSpace(r.StreetName) == "" || strings.TrimSpace(r.Building) == "" || strings.TrimSpace(r.Generation) == "" {
		return nil, status.Errorf(codes.DataLoss, "invalid subscription fields for chat %d", id)
	}
	return &r, nil
}

func (s *Server) save(id int64, r *record) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return status.Error(codes.Internal, "cannot encode subscription")
	}
	if err := writeFile(s.path(id), data); err != nil {
		return status.Error(codes.Internal, "cannot persist subscription")
	}
	return nil
}

func (r *record) subscription(id int64) *pb.Subscription {
	s := &pb.Subscription{ChatId: id, Address: &pb.Address{Street: &pb.Street{Id: r.StreetID, Name: r.StreetName}, Building: r.Building}, Generation: r.Generation}
	if r.StartDate != "" || r.EndDate != "" || r.Comment != "" {
		s.LastNotification = &pb.Notification{StartDate: r.StartDate, EndDate: r.EndDate, Comment: r.Comment}
	}
	return s
}

func validChat(id int64) error {
	if id == 0 {
		return invalidField("chat_id", "chat ID must not be zero")
	}
	return nil
}

func validGuard(g *pb.SubscriptionGuard) error {
	if strings.TrimSpace(g.GetGeneration()) == "" {
		return invalidField("expected.generation", "subscription generation is required")
	}
	return nil
}

func contextError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return status.FromContextError(err).Err()
	}
	return nil
}

func copyStreet(street *pb.Street) *pb.Street { return &pb.Street{Id: street.Id, Name: street.Name} }
