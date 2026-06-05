package domain

import "errors"

// ErrNotFound is returned when a requested resource does not exist. The
// transport layer maps it to HTTP 404.
var ErrNotFound = errors.New("resource not found")

// ErrConflict is returned when a request would violate a uniqueness or
// state-precondition invariant — e.g. inserting a row whose name collides
// with an existing one. Feature packages may wrap this with %w to add
// specificity (see exercise.ErrDuplicateName) while still being matchable
// against this sentinel via errors.Is. The transport layer maps it to HTTP
// 409 Conflict.
var ErrConflict = errors.New("resource conflict")

// ValidationError describes invalid input with a human-readable message. It is
// a distinct type so callers can use errors.As to recover the message; the
// transport layer maps it to HTTP 400.
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }
