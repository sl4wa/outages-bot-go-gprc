package domain

import "errors"

var (
	ErrFeedUnavailable = errors.New("outage feed unavailable")
	ErrInvalidSnapshot = errors.New("invalid outage snapshot")
)
