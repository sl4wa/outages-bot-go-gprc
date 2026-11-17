package domain

import "errors"

var (
	ErrFeedUnavailable  = errors.New("outage feed unavailable")
	ErrInvalidSnapshot  = errors.New("invalid outage snapshot")
	ErrSnapshotTooLarge = errors.New("outage snapshot too large")
)

type FeedError struct {
	Cause   error
	Message string
}

func (e *FeedError) Error() string { return e.Message }
func (e *FeedError) Unwrap() error { return e.Cause }
