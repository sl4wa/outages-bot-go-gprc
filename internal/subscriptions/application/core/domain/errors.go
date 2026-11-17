package domain

import "errors"

var ErrNotFound = errors.New("subscription not found")

type ValidationError struct {
	Field       string
	Description string
}

func (e *ValidationError) Error() string { return "invalid " + e.Field }

type DataLossError struct {
	Message string
}

func (e *DataLossError) Error() string { return e.Message }
