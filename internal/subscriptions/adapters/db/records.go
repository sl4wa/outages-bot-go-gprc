package db

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"outages-bot/internal/subscriptions/application/core/domain"
)

type record struct {
	StreetID   int32  `json:"street_id"`
	StreetName string `json:"street_name"`
	Building   string `json:"building"`
	Generation string `json:"generation"`
	StartDate  string `json:"start_date,omitempty"`
	EndDate    string `json:"end_date,omitempty"`
	Comment    string `json:"comment,omitempty"`
}

func (a *Adapter) path(id int64) string {
	return filepath.Join(a.usersDir, strconv.FormatInt(id, 10)+".json")
}

// Load durable records once. During operation, this process owns the directory
// and the in-memory map is authoritative; external file edits require restart.
func (a *Adapter) loadRecords() error {
	entries, err := os.ReadDir(a.usersDir)
	if err != nil {
		return fmt.Errorf("cannot read subscriptions directory")
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
		r, err := a.loadFile(id)
		if err != nil {
			return err
		}
		a.records[id] = *r
	}
	return nil
}

func (a *Adapter) loadFile(id int64) (*record, error) {
	data, err := os.ReadFile(a.path(id))
	if os.IsNotExist(err) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read subscription")
	}
	var r record
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, &domain.DataLossError{Message: fmt.Sprintf("invalid JSON for chat %d", id)}
	}
	if r.StreetID <= 0 || strings.TrimSpace(r.StreetName) == "" || strings.TrimSpace(r.Building) == "" || strings.TrimSpace(r.Generation) == "" {
		return nil, &domain.DataLossError{Message: fmt.Sprintf("invalid subscription fields for chat %d", id)}
	}
	return &r, nil
}

func (a *Adapter) save(id int64, r *record) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot encode subscription")
	}
	if err := writeFile(a.path(id), data); err != nil {
		return fmt.Errorf("cannot persist subscription")
	}
	return nil
}

func (r *record) subscription(id int64) domain.Subscription {
	s := domain.Subscription{
		ChatID: id, Address: domain.Address{Street: domain.Street{ID: r.StreetID, Name: r.StreetName}, Building: r.Building},
		Generation: r.Generation,
	}
	if r.StartDate != "" || r.EndDate != "" || r.Comment != "" {
		s.LastNotification = &domain.Notification{StartDate: r.StartDate, EndDate: r.EndDate, Comment: r.Comment}
	}
	return s
}

func recordFrom(s domain.Subscription) record {
	r := record{StreetID: s.Address.Street.ID, StreetName: s.Address.Street.Name, Building: s.Address.Building, Generation: s.Generation}
	if s.LastNotification != nil {
		r.StartDate, r.EndDate, r.Comment = s.LastNotification.StartDate, s.LastNotification.EndDate, s.LastNotification.Comment
	}
	return r
}
