package biometrics

import (
	"io"
	"net/http"

	"github.com/ataliaferro46/go-workout-api/internal/auth"
	"github.com/ataliaferro46/go-workout-api/internal/domain"
	"github.com/ataliaferro46/go-workout-api/internal/httpx"
)

// Handler serves the biometrics HTTP surface.
type Handler struct {
	svc *Service
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes registers the routes on mux. Auth-required routes are wrapped in
// requireAuth; the OAuth callback and webhook endpoints intentionally bypass
// it — callback uses the state token for binding, webhook uses HMAC.
func (h *Handler) Routes(mux *http.ServeMux, requireAuth func(http.Handler) http.Handler) {
	mux.Handle("GET /v1/biometrics/providers", requireAuth(http.HandlerFunc(h.providers)))
	mux.Handle("GET /v1/biometrics/connect/{provider}", requireAuth(http.HandlerFunc(h.connect)))
	mux.Handle("DELETE /v1/biometrics/connect/{provider}", requireAuth(http.HandlerFunc(h.disconnect)))
	mux.Handle("GET /v1/biometrics/latest", requireAuth(http.HandlerFunc(h.latest)))
	mux.Handle("POST /v1/biometrics/sync/{provider}", requireAuth(http.HandlerFunc(h.syncNow)))

	// Public — providers redirect/POST here without our cookie:
	mux.HandleFunc("GET /v1/biometrics/oauth/{provider}/callback", h.callback)
	mux.HandleFunc("POST /v1/biometrics/webhooks/{provider}", h.webhook)
}

func (h *Handler) syncNow(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	provider := trimProvider(r.PathValue("provider"))
	n, err := h.svc.SyncNow(r.Context(), u.ID, provider)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"provider":          provider,
		"readings_ingested": n,
	})
}

func (h *Handler) providers(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	out, err := h.svc.ListProviders(r.Context(), u.ID)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"providers": out})
}

func (h *Handler) connect(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	provider := trimProvider(r.PathValue("provider"))
	authURL, _, err := h.svc.Connect(r.Context(), u.ID, provider)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"auth_url": authURL})
}

func (h *Handler) disconnect(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	if err := h.svc.Disconnect(r.Context(), u.ID, trimProvider(r.PathValue("provider"))); err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusNoContent, nil)
}

func (h *Handler) callback(w http.ResponseWriter, r *http.Request) {
	provider := trimProvider(r.PathValue("provider"))
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	// The browser is following an OAuth redirect, not making an API call —
	// failures should land on the Settings page with a flash, successes
	// should land there with a "connected" badge. We never render raw JSON
	// here.
	if state == "" || code == "" {
		http.Redirect(w, r, "/settings?error=missing_params&provider="+provider, http.StatusFound)
		return
	}
	if err := h.svc.HandleCallback(r.Context(), state, code); err != nil {
		http.Redirect(w, r, "/settings?error=callback_failed&provider="+provider, http.StatusFound)
		return
	}
	http.Redirect(w, r, "/settings?connected="+provider, http.StatusFound)
}

func (h *Handler) latest(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	out, err := h.svc.LatestForUser(r.Context(), u.ID)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"readings": out})
}

func (h *Handler) webhook(w http.ResponseWriter, r *http.Request) {
	provider, err := h.svc.registry.Lookup(trimProvider(r.PathValue("provider")))
	if err != nil {
		httpx.Error(w, err)
		return
	}
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
