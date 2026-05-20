package domain

import "errors"

// ErrNotFound is returned when a requested resource does not exist.
// The transport layer maps it to HTTP 404.
var ErrNotFound = errors.New("resource not found")

// ValidationError describes an input validation failure with a
// human-readable message. The transport layer maps it to HTTP 400.
//
// It is a distinct type (rather than a sentinel error) so callers can use
// errors.As to recover the message and surface it to the client.
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }
