// Package httpx contains transport-layer helpers: JSON encoding, a stable
// error envelope, request decoding, and middleware. Keeping these here means
// feature handlers stay small and consistent.
package httpx

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
)

// JSON writes v as a JSON response with the given status code. A nil v
// writes only the status (useful for 204 No Content).
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// The header is already written, so we can't change the status;
		// just record that the body failed to serialize.
		slog.Error("failed to encode response body", "error", err)
	}
}

// errorBody is the stable error envelope returned to clients. Clients should
// branch on the machine-readable code, not the human-readable message.
type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// Error maps a domain error to an HTTP status code and writes the envelope.
// Unknown errors become 500 and are logged; their details are not leaked to
// the client.
func Error(w http.ResponseWriter, err error) {
	var (
		status        int
		code          string
		validationErr *domain.ValidationError
	)

	switch {
	case errors.As(err, &validationErr):
		status, code = http.StatusBadRequest, "validation_failed"
	case errors.Is(err, domain.ErrNotFound):
		status, code = http.StatusNotFound, "not_found"
	default:
		status, code = http.StatusInternalServerError, "internal_error"
		slog.Error("unhandled error", "error", err)
	}

	var body errorBody
	body.Error.Code = code
	if status == http.StatusInternalServerError {
		body.Error.Message = "an internal error occurred"
	} else {
		body.Error.Message = err.Error()
	}
	JSON(w, status, body)
}

// DecodeJSON decodes the request body into dst and rejects unknown fields,
// so typos and unexpected payloads fail loudly rather than silently.
func DecodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return &domain.ValidationError{Message: "invalid JSON body: " + err.Error()}
	}
	return nil
}
