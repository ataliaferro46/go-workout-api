package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"
)

// contextKey is unexported so no other package can collide with our context
// keys — the standard pattern for storing values in a context.
type contextKey string

const requestIDKey contextKey = "request_id"

// RequestID attaches a unique request ID to the context and echoes it in the
// response header. If the client supplied an X-Request-ID it is reused, which
// lets a trace span multiple services.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = newRequestID()
		}
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestIDFromContext returns the request ID stored in ctx, or "" if absent.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// statusRecorder wraps http.ResponseWriter to capture the status code so the
// logging middleware can record it.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Logger logs one structured line per request with method, path, status, and
// latency. Pulling the request ID from the context ties logs to traces.
func Logger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			log.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", RequestIDFromContext(r.Context()),
			)
		})
	}
}

// Recover turns a panic in any downstream handler into a 500 response and a
// logged error, so one bad request can't take down the whole process.
func Recover(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.Error("panic recovered",
						"error", rec,
						"request_id", RequestIDFromContext(r.Context()),
					)
					JSON(w, http.StatusInternalServerError, map[string]any{
						"error": map[string]string{
							"code":    "internal_error",
							"message": "an internal error occurred",
						},
					})
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// Chain applies middlewares so the first argument is the outermost wrapper.
// Chain(h, A, B) yields A(B(h)): A runs first on the way in, last on the way
// out.
func Chain(h http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

// newRequestID returns a 128-bit random hex string. crypto/rand failures are
// extremely unlikely; if one occurs we degrade to a placeholder rather than
// failing the request.
func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b[:])
}
