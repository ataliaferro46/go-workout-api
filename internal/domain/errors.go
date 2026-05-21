package domain

import "errors"

// ErrNotFound is returned when a requested resource does not exist. The
// transport layer maps it to HTTP 404.
var ErrNotFound = errors.New("resource not found")

// ValidationError describes invalid input with a human-readable message. It is
// a distinct type so callers can use errors.As to recover the message; the
// transport layer maps it to HTTP 400.
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }
