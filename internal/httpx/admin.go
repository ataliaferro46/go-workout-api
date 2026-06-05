package httpx

import (
	"crypto/subtle"
	"net/http"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
)

// AdminAuth gates a handler behind a static API key supplied via the
// ADMIN_API_KEY environment variable. It is a stopgap until JWT-based auth
// lands; the architecture is shaped so swapping it out is a localized change.
//
// Two safety properties matter:
//
//  1. Constant-time comparison. A naive `if got == expected` leaks timing
//     information; an attacker could enumerate the key one byte at a time by
//     measuring response latency. `subtle.ConstantTimeCompare` runs in time
//     proportional to the inputs, not to how many bytes matched.
//
//  2. Fail closed on missing config. If ADMIN_API_KEY is empty in the
//     environment, the middleware refuses every request. A forgotten config
//     value cannot accidentally open the admin surface to the world.
//
// The middleware sets WWW-Authenticate on rejection so HTTP clients see the
// realm explicitly; the response body uses the same envelope as other
// validation errors.
func AdminAuth(expectedKey string) func(http.Handler) http.Handler {
	if expectedKey == "" {
		// Configured-empty path: every request gets refused with a clear
		// message. We return a middleware that ignores `next` and writes a
		// 400 directly — the admin endpoints exist in the mux but are
		// effectively unreachable.
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Error(w, &domain.ValidationError{Message: "admin API is not configured on this server"})
			})
		}
	}

	// Cache the expected bytes once. `subtle.ConstantTimeCompare` requires
	// equal-length inputs to return 1; we explicitly check length first so
	// we don't leak whether the lengths matched via the underlying behavior.
	expected := []byte(expectedKey)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := []byte(r.Header.Get("X-Admin-API-Key"))
			if len(got) != len(expected) || subtle.ConstantTimeCompare(got, expected) != 1 {
				w.Header().Set("WWW-Authenticate", `Key realm="admin"`)
				Error(w, &domain.ValidationError{Message: "invalid or missing admin api key"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
