package workout

import (
	"net/http"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
	"github.com/ataliaferro46/go-workout-api/internal/httpx"
)

// Handler adapts HTTP requests to Service calls. It contains no business
// logic: it parses input, calls the service, and formats the response.
type Handler struct {
	svc *Service
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Routes registers the handler's routes on the given mux using Go 1.22's
// method-aware routing patterns.
func (h *Handler) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/workouts", h.create)
	mux.HandleFunc("GET /v1/workouts", h.list)
	mux.HandleFunc("GET /v1/workouts/{id}", h.get)
	mux.HandleFunc("DELETE /v1/workouts/{id}", h.delete)
}

// createRequest is the wire format for creating a workout. It is separate
// from CreateInput so the public API can change without touching the service.
type createRequest struct {
	Name      string            `json:"name"`
	Notes     string            `json:"notes"`
	Exercises []domain.Exercise `json:"exercises"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	// In a real service this would come from auth middleware that validates a
	// JWT and injects the user ID into the context. A header keeps the demo
	// self-contained while preserving per-user scoping.
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		httpx.Error(w, &domain.ValidationError{Message: "X-User-ID header is required"})
		return
	}

	var req createRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}

	out, err := h.svc.Create(r.Context(), CreateInput{
		UserID:    userID,
		Name:      req.Name,
		Notes:     req.Notes,
		Exercises: req.Exercises,
	})
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
	httpx.JSON(w, http.StatusOK, map[string]any{"workouts": out})
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), r.PathValue("id")); err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusNoContent, nil)
}
