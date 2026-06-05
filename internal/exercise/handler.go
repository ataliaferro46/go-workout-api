package exercise

import (
	"net/http"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
	"github.com/ataliaferro46/go-workout-api/internal/httpx"
)

// Handler serves the exercise library API. Public read endpoints anyone can
// hit; admin write endpoints are gated by the AdminAuth middleware that the
// caller (main.go) wraps them in.
type Handler struct {
	svc *Service
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Routes registers all the exercise routes onto mux. The adminAuth function
// is applied per-route around the admin write handlers — Go 1.22's ServeMux
// doesn't natively support per-route-group middleware, so we wrap at the
// HandleFunc layer instead. See ADR-063.
func (h *Handler) Routes(mux *http.ServeMux, adminAuth func(http.Handler) http.Handler) {
	// Public read endpoints.
	mux.HandleFunc("GET /v1/exercises", h.list)
	mux.HandleFunc("GET /v1/exercises/{id}", h.get)

	// Admin write endpoints. Each is wrapped individually so AdminAuth
	// cannot be accidentally bypassed by adding a new admin route without
	// the wrapper.
	mux.Handle("POST /v1/admin/exercises", adminAuth(http.HandlerFunc(h.adminCreate)))
	mux.Handle("PUT /v1/admin/exercises/{id}", adminAuth(http.HandlerFunc(h.adminUpdate)))
	mux.Handle("DELETE /v1/admin/exercises/{id}", adminAuth(http.HandlerFunc(h.adminDelete)))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.List(r.Context())
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"exercises": out})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) adminCreate(w http.ResponseWriter, r *http.Request) {
	var e domain.Exercise
	if err := httpx.DecodeJSON(r, &e); err != nil {
		httpx.Error(w, err)
		return
	}
	out, err := h.svc.Create(r.Context(), e)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, out)
}

func (h *Handler) adminUpdate(w http.ResponseWriter, r *http.Request) {
	var e domain.Exercise
	if err := httpx.DecodeJSON(r, &e); err != nil {
		httpx.Error(w, err)
		return
	}
	// Path id is canonical; body id (if present) is overwritten so clients
	// can't smuggle a different id through the body.
	e.ID = r.PathValue("id")
	out, err := h.svc.Update(r.Context(), e)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) adminDelete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), r.PathValue("id")); err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusNoContent, nil)
}
