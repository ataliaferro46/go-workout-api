package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
	"github.com/ataliaferro46/go-workout-api/internal/httpx"
)

// CookieName is the session cookie. Exported so tests and middleware share
// one source of truth.
const CookieName = "session"

// Handler exposes the auth HTTP surface.
type Handler struct {
	svc    *Service
	secure bool // sets Secure flag on the session cookie; true in production
}

// NewHandler constructs a Handler. Set secure=true when serving HTTPS
// (every request to fly.dev qualifies).
func NewHandler(svc *Service, secure bool) *Handler {
	return &Handler{svc: svc, secure: secure}
}

// Routes registers the auth endpoints.
//
//	POST   /v1/auth/signup           — create account + send verification email
//	GET    /v1/auth/verify           — consume verification token (also creates a session)
//	POST   /v1/auth/verify/resend    — resend verification email
//	POST   /v1/auth/login            — exchange email+password for session cookie
//	POST   /v1/auth/logout           — drop the current session
//	GET    /v1/auth/me               — return the authenticated user, or 401
func (h *Handler) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/auth/signup", h.signup)
	mux.HandleFunc("GET /v1/auth/verify", h.verify)
	mux.HandleFunc("POST /v1/auth/verify/resend", h.resend)
	mux.HandleFunc("POST /v1/auth/login", h.login)
	mux.HandleFunc("POST /v1/auth/logout", h.logout)
	mux.HandleFunc("GET /v1/auth/me", h.me)
	mux.HandleFunc("PATCH /v1/auth/profile", h.updateProfile)
}

func (h *Handler) updateProfile(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(CookieName)
	if err != nil || cookie.Value == "" {
		writeJSONUnauthorized(w, "not authenticated")
		return
	}
	u, err := h.svc.SessionUser(r.Context(), cookie.Value)
	if err != nil {
		writeJSONUnauthorized(w, "session expired")
		return
	}
	var req struct {
		HeightCM  *int     `json:"height_cm,omitempty"`
		WeightKG  *float64 `json:"weight_kg,omitempty"`
		BirthDate *string  `json:"birth_date,omitempty"` // YYYY-MM-DD
		Sex       *string  `json:"sex,omitempty"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	var birth *time.Time
	if req.BirthDate != nil && *req.BirthDate != "" {
		t, err := time.Parse("2006-01-02", *req.BirthDate)
		if err != nil {
			httpx.Error(w, &domain.ValidationError{Message: "birth_date must be YYYY-MM-DD"})
			return
		}
		birth = &t
	}
	updated, err := h.svc.UpdateProfile(r.Context(), u.ID, req.HeightCM, req.WeightKG, birth, req.Sex)
	if err != nil {
		mapAuthError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"user": updated})
}

type signupRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) signup(w http.ResponseWriter, r *http.Request) {
	var req signupRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	u, err := h.svc.Signup(r.Context(), req.Email, req.Password)
	if err != nil {
		mapAuthError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{
		"user":    u,
		"message": "Check your email for a verification link. It expires in 24 hours.",
	})
}

func (h *Handler) verify(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	u, err := h.svc.VerifyEmail(r.Context(), token)
	if err != nil {
		mapAuthError(w, err)
		return
	}
	// Auto-login on successful verification. The user clicked the link from
	// their email; signing them in immediately is the obvious UX.
	sess, _, err := h.svc.issueSessionFor(r.Context(), u, r.UserAgent(), clientIP(r))
	if err == nil {
		h.setSessionCookie(w, sess)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"user":    u,
		"message": "Email verified.",
	})
}

func (h *Handler) resend(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	if err := h.svc.ResendVerification(r.Context(), req.Email); err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusNoContent, nil)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req signupRequest // same shape: email + password
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	sess, u, err := h.svc.Login(r.Context(), req.Email, req.Password, r.UserAgent(), clientIP(r))
	if err != nil {
		mapAuthError(w, err)
		return
	}
	h.setSessionCookie(w, sess)
	httpx.JSON(w, http.StatusOK, map[string]any{"user": u})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(CookieName); err == nil {
		_ = h.svc.Logout(r.Context(), c.Value)
	}
	h.clearSessionCookie(w)
	httpx.JSON(w, http.StatusNoContent, nil)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	// /v1/auth/me is registered as a PUBLIC route (no RequireAuth wrapping)
	// because the front-end uses it as an "am I logged in?" probe — wrapping
	// it would bounce unauthenticated visitors and the login page itself
	// could never check session state. So we resolve the cookie inline
	// rather than reading from context (which is empty without middleware).
	cookie, err := r.Cookie(CookieName)
	if err != nil || cookie.Value == "" {
		writeJSONUnauthorized(w, "not authenticated")
		return
	}
	u, err := h.svc.SessionUser(r.Context(), cookie.Value)
	if err != nil {
		// Stale or invalid cookie — clear it so the browser stops resending.
		http.SetCookie(w, &http.Cookie{
			Name: CookieName, Value: "", Path: "/", MaxAge: -1,
			HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteLaxMode,
		})
		writeJSONUnauthorized(w, "session expired")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"user": u})
}

// writeJSONUnauthorized writes a 401 with the standard envelope. We don't
// route through httpx.Error here because that helper picks its own status
// code from the error type — `me` needs explicit 401 so the JS probe can
// distinguish "not logged in" from a transport failure.
func writeJSONUnauthorized(w http.ResponseWriter, msg string) {
	w.Header().Set("WWW-Authenticate", `Cookie realm="session"`)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{"code": "unauthorized", "message": msg},
	})
}

// --- helpers -----------------------------------------------------------

func (h *Handler) setSessionCookie(w http.ResponseWriter, sess Session) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    sess.Token,
		Path:     "/",
		Expires:  sess.ExpiresAt,
		HttpOnly: true,
		Secure:   h.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// issueSessionFor is exported just enough for the verify handler to log a
// user in immediately after they prove their email. It uses the existing
// Service.Login path's token-issuance logic without going through password
// verification.
func (s *Service) issueSessionFor(ctx context.Context, u User, ua, ip string) (Session, User, error) {
	now := time.Now().UTC()
	sess := Session{
		Token:     randomToken(),
		UserID:    u.ID,
		ExpiresAt: now.Add(s.sessionLifetime),
		CreatedAt: now,
		UserAgent: ua,
		IP:        ip,
	}
	sess.TokenHash = hashToken(sess.Token)
	if err := s.sessions.Insert(ctx, sess); err != nil {
		return Session{}, User{}, err
	}
	return sess, u, nil
}

func clientIP(r *http.Request) string {
	// Fly forwards the real client IP in Fly-Client-IP. X-Forwarded-For is
	// the proxy-standard fallback.
	if ip := r.Header.Get("Fly-Client-IP"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		if comma := strings.IndexByte(ip, ','); comma >= 0 {
			return strings.TrimSpace(ip[:comma])
		}
		return strings.TrimSpace(ip)
	}
	host, _, _ := splitHostPort(r.RemoteAddr)
	return host
}

func splitHostPort(addr string) (string, string, error) {
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		return addr[:i], addr[i+1:], nil
	}
	return addr, "", nil
}

func mapAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrEmailTaken):
		httpx.Error(w, &domain.ValidationError{Message: "email already registered"})
	case errors.Is(err, ErrInvalidLogin):
		w.Header().Set("WWW-Authenticate", `Cookie realm="session"`)
		httpx.Error(w, &domain.ValidationError{Message: "invalid email or password"})
	case errors.Is(err, ErrEmailNotVerified):
		httpx.Error(w, &domain.ValidationError{Message: "please verify your email first"})
	case errors.Is(err, ErrTokenInvalid):
		httpx.Error(w, &domain.ValidationError{Message: "token invalid or expired"})
	case errors.Is(err, ErrSessionExpired):
		httpx.Error(w, &domain.ValidationError{Message: "session expired; please log in again"})
	case errors.Is(err, ErrUserNotFound):
		httpx.Error(w, &domain.ValidationError{Message: "user not found"})
	default:
		var vErr *ValidationError
		if errors.As(err, &vErr) {
			httpx.Error(w, &domain.ValidationError{Message: vErr.Message})
			return
		}
		httpx.Error(w, err)
	}
}
