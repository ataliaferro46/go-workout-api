package workout

import (
	"context"
	"net/http"
	"time"

	"github.com/ataliaferro46/go-workout-api/internal/auth"
	"github.com/ataliaferro46/go-workout-api/internal/domain"
	"github.com/ataliaferro46/go-workout-api/internal/httpx"
)

// IntensityFetcher returns an HR-derived intensity summary for a window.
// Injected so the workout handler doesn't depend on the biometrics
// package directly; nil disables the endpoint cleanly.
type IntensityFetcher interface {
	IntensityForWindow(ctx context.Context, userID string, start, end time.Time) (any, error)
}

// Handler adapts HTTP requests to Service calls for logged workouts.
type Handler struct {
	svc       *Service
	intensity IntensityFetcher
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// SetIntensityFetcher attaches an intensity source. Call from main.go
// after constructing the biometrics service.
func (h *Handler) SetIntensityFetcher(f IntensityFetcher) {
	h.intensity = f
}

// Routes registers all workout routes behind requireAuth.
func (h *Handler) Routes(mux *http.ServeMux, requireAuth func(http.Handler) http.Handler) {
	mux.Handle("POST /v1/workouts", requireAuth(http.HandlerFunc(h.create)))
	mux.Handle("GET /v1/workouts", requireAuth(http.HandlerFunc(h.list)))
	mux.Handle("GET /v1/workouts/{id}", requireAuth(http.HandlerFunc(h.get)))
	mux.Handle("DELETE /v1/workouts/{id}", requireAuth(http.HandlerFunc(h.delete)))
	mux.Handle("POST /v1/workouts/{id}/sets", requireAuth(http.HandlerFunc(h.logSet)))
	mux.Handle("GET /v1/workouts/{id}/intensity", requireAuth(http.HandlerFunc(h.intensityHandler)))
	mux.Handle("GET /v1/workouts/exercise/last", requireAuth(http.HandlerFunc(h.lastForExercise)))
	mux.Handle("POST /v1/workouts/cardio", requireAuth(http.HandlerFunc(h.createCardio)))
	mux.Handle("POST /v1/workouts/{id}/repeat", requireAuth(http.HandlerFunc(h.repeat)))
}

// UserWeightFetcher lets the cardio handler ask for the auth user's
// stored body weight without importing auth directly. Injected.
type UserWeightFetcher func(ctx context.Context, userID string) (float64, error)

var weightFetcher UserWeightFetcher

// SetUserWeightFetcher wires the auth-profile weight lookup.
func (h *Handler) SetUserWeightFetcher(f UserWeightFetcher) { weightFetcher = f }

func (h *Handler) createCardio(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	var req struct {
		Name            string  `json:"name"`
		Notes           string  `json:"notes"`
		Activity        string  `json:"activity"`
		Intensity       string  `json:"intensity"`
		DurationMinutes int     `json:"duration_minutes"`
		DistanceKM      float64 `json:"distance_km,omitempty"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	var weight float64
	if weightFetcher != nil {
		if wgt, _ := weightFetcher(r.Context(), u.ID); wgt > 0 {
			weight = wgt
		}
	}
	out, err := h.svc.CreateCardio(r.Context(), CardioInput{
		UserID:          u.ID,
		Name:            req.Name,
		Notes:           req.Notes,
		Activity:        req.Activity,
		Intensity:       req.Intensity,
		DurationMinutes: req.DurationMinutes,
		DistanceKM:      req.DistanceKM,
		UserWeightKG:    weight,
	})
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, out)
}

func (h *Handler) repeat(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	src, err := h.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.Error(w, err)
		return
	}
	if src.UserID != u.ID {
		httpx.Error(w, domain.ErrNotFound)
		return
	}
	if src.Type == "cardio" && src.CardioSession != nil {
		out, err := h.svc.CreateCardio(r.Context(), CardioInput{
			UserID:          u.ID,
			Name:            src.Name,
			Activity:        src.CardioSession.Activity,
			Intensity:       src.CardioSession.Intensity,
			DurationMinutes: src.CardioSession.DurationMinutes,
			DistanceKM:      src.CardioSession.DistanceKM,
		})
		if err != nil {
			httpx.Error(w, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, out)
		return
	}
	// Strength: clone the prescription, no logged sets.
	clone := CreateInput{
		UserID:     u.ID,
		Name:       src.Name,
		Notes:      src.Notes,
		PlanID:     src.PlanID,
		PlanDayIdx: src.PlanDayIdx,
		Exercises:  make([]domain.LoggedExercise, 0, len(src.Exercises)),
	}
	for _, ex := range src.Exercises {
		clone.Exercises = append(clone.Exercises, domain.LoggedExercise{
			Name:     ex.Name,
			Sets:     ex.Sets,
			Reps:     ex.Reps,
			WeightKG: ex.WeightKG,
		})
	}
	out, err := h.svc.Create(r.Context(), clone)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, out)
}

func (h *Handler) lastForExercise(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	name := r.URL.Query().Get("name")
	if name == "" {
		httpx.Error(w, &domain.ValidationError{Message: "name query param required"})
		return
	}
	sets, when, err := h.svc.LastSetsForExercise(r.Context(), u.ID, name)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	out := map[string]any{"sets": sets}
	if !when.IsZero() {
		out["when"] = when
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) intensityHandler(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	id := r.PathValue("id")
	out, err := h.svc.Get(r.Context(), id)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	if out.UserID != u.ID {
		httpx.Error(w, domain.ErrNotFound)
		return
	}
	if h.intensity == nil {
		httpx.JSON(w, http.StatusOK, map[string]any{"intensity": nil, "available": false})
		return
	}
	// Time window: workout start (created_at) to the most recent logged set,
	// or +60min from start if nothing's logged yet.
	start := out.CreatedAt
	end := start.Add(60 * time.Minute)
	for _, ex := range out.Exercises {
		for _, ls := range ex.LoggedSets {
			if ls.CompletedAt.After(end) {
				end = ls.CompletedAt
			}
		}
	}
	intensity, err := h.intensity.IntensityForWindow(r.Context(), u.ID, start, end)
	if err != nil {
		// Don't 500 — present as "no data". Logging would be nice here but
		// pollutes the response.
		httpx.JSON(w, http.StatusOK, map[string]any{"intensity": nil, "available": true, "error": err.Error()})
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"intensity": intensity, "available": true})
}

func (h *Handler) logSet(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	id := r.PathValue("id")
	var req struct {
		ExercisePosition int     `json:"exercise_position"`
		SetNumber        int     `json:"set_number"`
		Reps             int     `json:"reps"`
		WeightKG         float64 `json:"weight_kg"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	// Ownership check.
	existing, err := h.svc.Get(r.Context(), id)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	if existing.UserID != u.ID {
		httpx.Error(w, domain.ErrNotFound)
		return
	}
	if err := h.svc.LogSet(r.Context(), id, req.ExercisePosition, req.SetNumber, req.Reps, req.WeightKG); err != nil {
		httpx.Error(w, err)
		return
	}
	out, err := h.svc.Get(r.Context(), id)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

// createRequest is the wire format for logging a workout.
type createRequest struct {
	Name       string                  `json:"name"`
	Notes      string                  `json:"notes"`
	PlanID     string                  `json:"plan_id,omitempty"`
	PlanDayIdx int                     `json:"plan_day_idx,omitempty"`
	Exercises  []domain.LoggedExercise `json:"exercises"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())

	var req createRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}

	out, err := h.svc.Create(r.Context(), CreateInput{
		UserID:     u.ID,
		Name:       req.Name,
		Notes:      req.Notes,
		PlanID:     req.PlanID,
		PlanDayIdx: req.PlanDayIdx,
		Exercises:  req.Exercises,
	})
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, out)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	out, err := h.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.Error(w, err)
		return
	}
	if out.UserID != u.ID {
		httpx.Error(w, domain.ErrNotFound)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	out, err := h.svc.ListByUser(r.Context(), u.ID)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"workouts": out})
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	id := r.PathValue("id")
	w0, err := h.svc.Get(r.Context(), id)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	if w0.UserID != u.ID {
		httpx.Error(w, domain.ErrNotFound)
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusNoContent, nil)
}
