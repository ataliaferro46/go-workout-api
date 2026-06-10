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
	mux.HandleFunc("GET /v1/exercises/alternatives", h.alternativesByName)
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

// alternativesByName finds exercises similar to the one whose name is
// given via ?name=. "Similar" = same primary muscle and, when the source
// is a compound, same movement pattern. Returns up to 6 ranked results.
// Plan-agnostic so the live workout page can use it without needing a
// plan context.
func (h *Handler) alternativesByName(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		httpx.Error(w, &domain.ValidationError{Message: "name query param required"})
		return
	}
	all, err := h.svc.List(r.Context())
	if err != nil {
		httpx.Error(w, err)
		return
	}
	// Find the source exercise. Case-insensitive match by name.
	var src *domain.Exercise
	nameLower := lowerString(name)
	for i, e := range all {
		if lowerString(e.Name) == nameLower {
			src = &all[i]
			break
		}
	}
	if src == nil {
		httpx.JSON(w, http.StatusOK, map[string]any{"alternatives": []domain.Exercise{}})
		return
	}
	// Gather candidates with same primary muscle. For compounds also
	// require same movement pattern so a bench-press alt isn't a fly.
	out := make([]domain.Exercise, 0, 12)
	for _, e := range all {
		if e.ID == src.ID {
			continue
		}
		if e.PrimaryMuscle != src.PrimaryMuscle {
			continue
		}
		if src.Compound && e.Pattern != src.Pattern {
			continue
		}
		out = append(out, e)
		if len(out) >= 18 {
			break
		}
	}
	// Rank: same pattern wins, then same compound flag, then matching region.
	rankByCloseness(out, *src)
	if len(out) > 6 {
		out = out[:6]
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"alternatives": out})
}

func rankByCloseness(cands []domain.Exercise, target domain.Exercise) {
	score := func(c domain.Exercise) int {
		s := 0
		if c.Pattern == target.Pattern {
			s += 10
		}
		if c.Compound == target.Compound {
			s += 5
		}
		if c.Region != "" && c.Region == target.Region {
			s += 3
		}
		if c.MinLevel == target.MinLevel {
			s += 1
		}
		return s
	}
	for i := 1; i < len(cands); i++ {
		j := i
		for j > 0 && score(cands[j]) > score(cands[j-1]) {
			cands[j], cands[j-1] = cands[j-1], cands[j]
			j--
		}
	}
}

func lowerString(s string) string {
	out := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		out[i] = c
	}
	return string(out)
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
