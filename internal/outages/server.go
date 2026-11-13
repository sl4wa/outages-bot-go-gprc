// Package outages owns the external LOE feed integration. It never reads
// subscription files or calls Telegram.
package outages

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	outagesv1 "outages-bot/gen/outages/v1"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const maxFeedBytes = 8 << 20

type Server struct {
	outagesv1.UnimplementedOutageServiceServer
	url    string
	client *http.Client
}

func New(url string, client *http.Client) *Server {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	return &Server{url: url, client: client}
}

// FetchSnapshot retrieves a fresh outage snapshot for every call.
func (s *Server) FetchSnapshot(ctx context.Context, _ *outagesv1.FetchSnapshotRequest) (*outagesv1.FetchSnapshotResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
	if err != nil {
		return nil, status.Error(codes.Internal, "invalid outage API configuration")
	}
	req.Header.Set("Accept", "application/json")
	res, err := s.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, status.FromContextError(ctx.Err()).Err()
		}
		return nil, status.Error(codes.Unavailable, "outage API request failed")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, status.Errorf(codes.Unavailable, "outage API returned HTTP %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, maxFeedBytes+1))
	if err != nil {
		if ctx.Err() != nil {
			return nil, status.FromContextError(ctx.Err()).Err()
		}
		return nil, status.Error(codes.Unavailable, "reading outage API response failed")
	}
	if len(body) > maxFeedBytes {
		return nil, status.Error(codes.ResourceExhausted, "outage API response exceeds 8 MiB")
	}
	rows, err := normalize(body)
	if err != nil {
		return nil, status.Error(codes.DataLoss, "outage API returned an invalid snapshot")
	}
	return &outagesv1.FetchSnapshotResponse{Outages: rows}, nil
}

// The API returns integer street IDs and comma-separated building names.
// Unexpected scalar types are invalid rows.
type rawOutage struct {
	Start string `json:"dateEvent"`
	End   string `json:"datePlanIn"`
	City  struct {
		Name string `json:"name"`
	} `json:"city"`
	Street struct {
		ID   int32  `json:"id"`
		Name string `json:"name"`
	} `json:"street"`
	Buildings string `json:"buildingNames"`
	Comment   string `json:"koment"`
}

func clean(value string) string { return strings.Join(strings.Fields(value), " ") }

func normalize(body []byte) ([]*outagesv1.Outage, error) {
	body = bytes.TrimSpace(body)
	// The provider may return a JSON-LD collection even for application/json.
	if len(body) > 0 && body[0] == '{' {
		var collection struct {
			Members json.RawMessage `json:"hydra:member"`
		}
		if err := json.Unmarshal(body, &collection); err != nil {
			return nil, err
		}
		body = bytes.TrimSpace(collection.Members)
	}
	if len(body) == 0 || body[0] != '[' {
		return nil, errors.New("expected an outage array")
	}
	var members []json.RawMessage
	if err := json.Unmarshal(body, &members); err != nil {
		return nil, err
	}
	result := make([]*outagesv1.Outage, 0, len(members))
	for _, member := range members {
		var raw rawOutage
		if json.Unmarshal(member, &raw) != nil {
			continue
		}
		row := &outagesv1.Outage{
			StartDate: clean(raw.Start), EndDate: clean(raw.End), City: clean(raw.City.Name),
			StreetId: raw.Street.ID, StreetName: clean(raw.Street.Name), Comment: clean(raw.Comment),
		}
		for building := range strings.SplitSeq(raw.Buildings, ",") {
			if building = strings.TrimSpace(building); building != "" {
				row.Buildings = append(row.Buildings, building)
			}
		}
		if row.StreetId <= 0 || row.StartDate == "" || row.EndDate == "" || row.StreetName == "" || len(row.Buildings) == 0 {
			continue
		}
		result = append(result, row)
	}
	if skipped := len(members) - len(result); skipped > 0 {
		slog.Warn("skipped invalid outage rows", "count", skipped)
		if len(result) == 0 {
			return nil, errors.New("no valid outage rows")
		}
	}
	return result, nil
}
