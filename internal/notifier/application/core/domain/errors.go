package domain

import "errors"

var (
	ErrRunInProgress = errors.New("a notification run is already in progress")
	ErrRejected      = errors.New("notification rejected")
	ErrBlocked       = errors.New("recipient blocked delivery")
	ErrDelivery      = errors.New("Telegram delivery failed; prior deliveries remain acknowledged")
)

type RateLimitError struct {
	RetryAfter int
}

func (e *RateLimitError) Error() string { return "Telegram rate limit; retry on a later run" }
