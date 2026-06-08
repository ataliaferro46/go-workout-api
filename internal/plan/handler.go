package plan

import (
	"net/http"
	"strconv"
	"time"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
	"github.com/ataliaferro46/go-workout-api/internal/httpx"
)

// Handler serves the plan generation + retrieval API. It holds the Service,
// not the exercise library directly — persistence and metadata stamping live
// in the Service, transport-only concerns live here.
type Handler struct {
	svc *Service
}

// NewHandler returns a Handler over the given Service.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Routes registers the handler's routes on the mux.
func (h *Handler) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/plans/generate", h.generate)
	mux.HandleFunc("GET /v1/plans", h.list)
	mux.HandleFunc("GET /v1/plans/{id}", h.get)
	mux.HandleFunc("DELETE /v1/plans/{id}", h.delete)
}

// generate runs the engine for the requesting user and persists the plan. An
// optional ?seed= makes the result reproducible; otherwise we vary by
// wall-clock time. The user is identified by the X-User-ID header — a
// placeholder for real auth (see ARCHITECTURE.md "Deferred").
func (h *Handler) generate(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		httpx.Error(w, &domain.ValidationError{Message: "X-User-ID header is required"})
		return
	}

	var req domain.GenerateRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}

	seed := time.Now().UnixNano()
	if s := r.URL.Query().Get("seed"); s != "" {
		parsed, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			httpx.Error(w, &domain.ValidationError{Message: "seed must be an integer"})
			return
		}
		seed = parsed
	}

	// ?recovery_aware=true opts in to recovery-biased generation. Default
	// is off so the endpoint stays predictable for clients that don't yet
	// integrate biometrics.
	recoveryAware := r.URL.Query().Get("recovery_aware") == "true"

	out, err := h.svc.Create(r.Context(), userID, req, seed, recoveryAware)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, out)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.ListByUser(r.Context(), r.Header.Get("X-User-ID"))
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"plans": out})
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), r.PathValue("id")); err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusNoContent, nil)
}
