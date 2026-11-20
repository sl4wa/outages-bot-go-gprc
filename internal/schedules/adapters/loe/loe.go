// Package loe adapts the external LOE schedule HTTP feed to schedule domain values.
package loe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"outages-bot/internal/schedules/application/core/domain"
	"outages-bot/internal/schedules/ports"
)

const maxFeedBytes = 8 << 20

var (
	dateRE    = regexp.MustCompile(`\d{2}\.\d{2}\.\d{4}`)
	updatedRE = regexp.MustCompile(`\d{2}:\d{2}\s+\d{2}\.\d{2}\.\d{4}`)
	blockRE   = regexp.MustCompile(`(?i)</?\s*(p|div|br)[^>]*>`)
	tagRE     = regexp.MustCompile(`<[^>]*>`)
)

type Adapter struct {
	url    string
	client *http.Client
}

func NewAdapter(url string, client *http.Client) *Adapter {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	return &Adapter{url: url, client: client}
}

var _ ports.FeedPort = (*Adapter)(nil)

func (a *Adapter) Fetch(ctx context.Context) ([]domain.Schedule, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.url, nil)
	if err != nil {
		return nil, errors.New("invalid schedule API configuration")
	}
	req.Header.Set("Accept", "application/json")
	res, err := a.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, domain.ErrFeedUnavailable
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: HTTP %d", domain.ErrFeedUnavailable, res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, maxFeedBytes))
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, domain.ErrFeedUnavailable
	}
	schedules, err := normalize(body)
	if err != nil {
		return nil, domain.ErrInvalidFeed
	}
	return schedules, nil
}

type menuItem struct {
	RawHTML     string     `json:"rawHtml"`
	Description string     `json:"description"`
	Children    []menuItem `json:"children"`
}

type menu struct {
	MenuItems []menuItem `json:"menuItems"`
}

func normalize(body []byte) ([]domain.Schedule, error) {
	// The API returns either a bare array or a JSON-LD hydra:member wrapper.
	// Neither `null` nor a missing hydra:member counts as an empty feed.
	var menus []menu
	if err := json.Unmarshal(body, &menus); err != nil || menus == nil {
		var wrapped struct {
			Members []menu `json:"hydra:member"`
		}
		if err := json.Unmarshal(body, &wrapped); err != nil {
			return nil, err
		}
		if wrapped.Members == nil {
			return nil, errors.New("no schedule menus in response")
		}
		menus = wrapped.Members
	}
	var schedules []domain.Schedule
	var add func(menuItem)
	add = func(m menuItem) {
		if s, ok := toSchedule(m); ok {
			schedules = append(schedules, s)
		}
		for _, child := range m.Children {
			add(child)
		}
	}
	for _, menu := range menus {
		for _, m := range menu.MenuItems {
			add(m)
		}
	}
	return schedules, nil
}

func toSchedule(m menuItem) (domain.Schedule, bool) {
	raw := m.RawHTML
	if strings.TrimSpace(raw) == "" {
		raw = m.Description
	}
	text := plainText(raw)
	// Provider times are local wall time; keep them without a zone, as outages does.
	date, err1 := time.Parse("02.01.2006", dateRE.FindString(text))
	updated, err2 := time.Parse("15:04 02.01.2006", strings.Join(strings.Fields(updatedRE.FindString(text)), " "))
	if err1 != nil || err2 != nil {
		return domain.Schedule{}, false
	}
	var lines []string
	for line := range strings.SplitSeq(text, "\n") {
		// The title line only repeats the date.
		if !strings.HasPrefix(line, "Графік погодинних") {
			lines = append(lines, line)
		}
	}
	return domain.Schedule{Date: date.Format("2006-01-02"), Updated: updated.Format("2006-01-02T15:04"), Lines: lines}, true
}

// plainText turns the provider's HTML fragment into trimmed non-empty lines.
func plainText(raw string) string {
	text := html.UnescapeString(tagRE.ReplaceAllString(blockRE.ReplaceAllString(raw, "\n"), ""))
	var lines []string
	for line := range strings.SplitSeq(text, "\n") {
		if line = strings.Join(strings.Fields(line), " "); line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}
