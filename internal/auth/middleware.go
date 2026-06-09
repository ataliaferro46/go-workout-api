package auth

import (
	"context"
	"encoding/json"
	"net/http"
)

// contextKey isolates auth's context entries from other packages' keys.
type contextKey int

const (
	ctxKeyUser contextKey = iota
)

// WithUser returns a context carrying u. Used by both the middleware
// (production) and tests (which can skip the middleware and inject directly).
func WithUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, ctxKeyUser, u)
}

// UserFromContext returns the authenticated user, if any.
func UserFromContext(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(ctxKeyUser).(User)
	return u, ok
}

// RequireAuth wraps a handler so that requests without a valid session
// cookie are rejected with 401. On success, the user is attached to the
// request context via WithUser; downstream handlers retrieve it via
// UserFromContext.
//
// Used as a middleware around protected route subsets in main.go. Pages and
// /v1/auth/* should NOT be wrapped (login flows must work for the
// unauthenticated user).
func RequireAuth(svc *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(CookieName)
			if err != nil || cookie.Value == "" {
				unauthorized(w)
				return
			}
			u, err := svc.SessionUser(r.Context(), cookie.Value)
			if err != nil {
				// Stale or invalid cookie — clear it so the browser doesn't
				// keep re-sending dead tokens.
				http.SetCookie(w, &http.Cookie{
					Name: CookieName, Value: "", Path: "/", MaxAge: -1,
					HttpOnly: true, SameSite: http.SameSiteLaxMode,
				})
				unauthorized(w)
				return
			}
			next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), u)))
		})
	}
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Cookie realm="session"`)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{
			"code":    "unauthorized",
			"message": "not authenticated",
		},
	})
}
