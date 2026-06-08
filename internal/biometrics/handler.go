package biometrics

import (
	"io"
	"net/http"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
	"github.com/ataliaferro46/go-workout-api/internal/httpx"
)

// Handler serves the biometrics HTTP surface.
type Handler struct {
	svc *Service
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes registers the routes on mux.
//
// /v1/biometrics/connect/{provider}        — start OAuth (X-User-ID)
// /v1/biometrics/oauth/{provider}/callback — OAuth redirect target
// /v1/biometrics/latest                    — latest reading per kind (X-User-ID)
// /v1/biometrics/webhooks/{provider}       — webhook receiver (HMAC sig)
// /v1/biometrics/connect/{provider}        — DELETE: disconnect (X-User-ID)
func (h *Handler) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/biometrics/connect/{provider}", h.connect)
	mux.HandleFunc("DELETE /v1/biometrics/connect/{provider}", h.disconnect)
	mux.HandleFunc("GET /v1/biometrics/oauth/{provider}/callback", h.callback)
	mux.HandleFunc("GET /v1/biometrics/latest", h.latest)
	mux.HandleFunc("POST /v1/biometrics/webhooks/{provider}", h.webhook)
}

func (h *Handler) connect(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		httpx.Error(w, &domain.ValidationError{Message: "X-User-ID header is required"})
		return
	}
	provider := trimProvider(r.PathValue("provider"))
	authURL, _, err := h.svc.Connect(r.Context(), userID, provider)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"auth_url": authURL})
}

func (h *Handler) disconnect(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		httpx.Error(w, &domain.ValidationError{Message: "X-User-ID header is required"})
		return
	}
	if err := h.svc.Disconnect(r.Context(), userID, trimProvider(r.PathValue("provider"))); err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusNoContent, nil)
}

func (h *Handler) callback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	if state == "" || code == "" {
		httpx.Error(w, &domain.ValidationError{Message: "state and code are required"})
		return
	}
	if err := h.svc.HandleCallback(r.Context(), state, code); err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "connected"})
}

func (h *Handler) latest(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		httpx.Error(w, &domain.ValidationError{Message: "X-User-ID header is required"})
		return
	}
	out, err := h.svc.LatestForUser(r.Context(), userID)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"readings": out})
}

// webhook verifies the HMAC signature and returns 200. Actual ingestion is
// done by the polling daemon; the webhook acts as a freshness signal so
// providers don't keep retrying. See ADR-049 — this is an intentional
// simplification for v1; full webhook ingestion is one Provider method
// (ParseWebhook) and one Service call away.
func (h *Handler) webhook(w http.ResponseWriter, r *http.Request) {
	provider, err := h.svc.registry.Lookup(trimProvider(r.PathValue("provider")))
	if err != nil {
		httpx.Error(w, err)
		return
	}
	// Cap body size before reading — webhook payloads are small and
	// unbounded reads are a DoS vector.
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<16))
	if err != nil {
		httpx.Error(w, &domain.ValidationError{Message: "request body too large"})
		return
	}
	if err := provider.VerifyWebhook(r.Header, body); err != nil {
		w.Header().Set("WWW-Authenticate", `Sig realm="webhook"`)
		httpx.Error(w, &domain.ValidationError{Message: "signature verification failed"})
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
