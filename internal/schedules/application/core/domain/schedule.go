package domain

import "errors"

// Schedule is one published hourly-outage schedule. Date and Updated are
// lexically sortable (YYYY-MM-DD and YYYY-MM-DDTHH:MM, provider local time).
type Schedule struct {
	Date    string
	Updated string
	Lines   []string
}

var (
	ErrFeedUnavailable = errors.New("schedule feed unavailable")
	ErrInvalidFeed     = errors.New("invalid schedule feed")
)
