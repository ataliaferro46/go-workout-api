package plan

import (
	"net/http"
	"strconv"
	"time"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
	"github.com/ataliaferro46/go-workout-api/internal/httpx"
)

// Handler serves the plan generation API. It holds the immutable exercise
// library and constructs a fresh Generator per request, which keeps request
// handling free of shared mutable RNG state — so it's safe under the server's
// concurrent request handling without any locking.
type Handler struct {
	library []domain.Exercise
}

// NewHandler returns a Handler over the given exercise library.
func NewHandler(library []domain.Exercise) *Handler {
	return &Handler{library: library}
}

// Routes registers the handler's routes on the mux.
func (h *Handler) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/plans/generate", h.generate)
}

func (h *Handler) generate(w http.ResponseWriter, r *http.Request) {
	var req domain.GenerateRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}

	// An optional ?seed= makes a plan reproducible (useful for testing and for
	// "regenerate the same plan"); otherwise we vary by wall-clock time.
	seed := time.Now().UnixNano()
	if s := r.URL.Query().Get("seed"); s != "" {
		parsed, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			httpx.Error(w, &domain.ValidationError{Message: "seed must be an integer"})
			return
		}
		seed = parsed
	}

	out, err := NewGenerator(h.library, seed).Generate(req)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}
