package plan

import (
	"net/http"
	"strconv"
	"time"

	"github.com/ataliaferro46/go-workout-api/internal/auth"
	"github.com/ataliaferro46/go-workout-api/internal/domain"
	"github.com/ataliaferro46/go-workout-api/internal/httpx"
)

// Handler serves the plan generation + retrieval API. The authenticated user
// is read from request context — the RequireAuth middleware (applied in
// Routes) ensures it's always present.
type Handler struct {
	svc *Service
}

// NewHandler returns a Handler over the given Service.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Routes registers the handler's routes. Every endpoint is wrapped in
// requireAuth so unauthenticated callers receive 401 before any handler
// logic runs.
func (h *Handler) Routes(mux *http.ServeMux, requireAuth func(http.Handler) http.Handler) {
	mux.Handle("POST /v1/plans/generate", requireAuth(http.HandlerFunc(h.generate)))
	mux.Handle("GET /v1/plans", requireAuth(http.HandlerFunc(h.list)))
	mux.Handle("GET /v1/plans/{id}", requireAuth(http.HandlerFunc(h.get)))
	mux.Handle("DELETE /v1/plans/{id}", requireAuth(http.HandlerFunc(h.delete)))
	mux.Handle("GET /v1/plans/{id}/days/{day}/exercises/{order}/alternatives", requireAuth(http.HandlerFunc(h.alternatives)))
	mux.Handle("PATCH /v1/plans/{id}/days/{day}/exercises/{order}", requireAuth(http.HandlerFunc(h.swap)))
	mux.Handle("PATCH /v1/plans/{id}/days/reorder", requireAuth(http.HandlerFunc(h.reorder)))
	mux.Handle("POST /v1/plans/quick-day", requireAuth(http.HandlerFunc(h.quickDay)))
}

// quickDay returns a single ad-hoc training day based on user-picked
// muscle groups + the standard generation filters. Accepts the same
// multi-goal + recovery-aware controls as the weekly plan generator.
// The caller (front-end) then POSTs the returned exercises to
// /v1/workouts to start the session.
func (h *Handler) quickDay(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	var req struct {
		Goal           string   `json:"goal"`
		SecondaryGoals []string `json:"secondary_goals,omitempty"`
		Experience     string   `json:"experience"`
		Equipment      []string `json:"available_equipment"`
		Injuries       []string `json:"injuries"`
		Muscles        []string `json:"muscles"`
		SessionMinutes int      `json:"session_minutes"`
		SetsOverride   *int     `json:"sets_override,omitempty"`
		RecoveryHint   *float64 `json:"recovery_hint,omitempty"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	qreq := QuickDayRequest{
		Goal:           domain.Goal(req.Goal),
		Experience:     domain.ExperienceLevel(req.Experience),
		SessionMinutes: req.SessionMinutes,
		SetsOverride:   req.SetsOverride,
		RecoveryHint:   req.RecoveryHint,
	}
	for _, e := range req.Equipment {
		qreq.Equipment = append(qreq.Equipment, domain.Equipment(e))
	}
	for _, i := range req.Injuries {
		qreq.Injuries = append(qreq.Injuries, domain.BodyPart(i))
	}
	for _, m := range req.Muscles {
		qreq.Muscles = append(qreq.Muscles, domain.MuscleGroup(m))
	}
	// Recovery-aware: same opt-in as the weekly generator — if true and the
	// service has a recovery source, it overwrites RecoveryHint from the
	// user's latest biometric reading.
	recoveryAware := r.URL.Query().Get("recovery_aware") == "true"
	if recoveryAware {
		if rec := h.svc.LatestRecoveryHint(r.Context(), u.ID); rec != nil {
			qreq.RecoveryHint = rec
		}
	}
	day, err := h.svc.BuildAdHocDay(r.Context(), qreq)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	// secondary_goals is accepted but not (yet) used by the engine — we
	// echo it back on the day so the front-end can display "running both
	// fat loss + muscle gain priorities."
	_ = req.SecondaryGoals
	httpx.JSON(w, http.StatusOK, day)
}

func (h *Handler) reorder(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	planID := r.PathValue("id")
	var req struct {
		Order []int `json:"order"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	// Ownership check.
	p, err := h.svc.Get(r.Context(), planID)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	if p.UserID != u.ID {
		httpx.Error(w, domain.ErrNotFound)
		return
	}
	out, err := h.svc.ReorderDays(r.Context(), planID, req.Order)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) alternatives(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	planID := r.PathValue("id")
	dayIdx, err := strconv.Atoi(r.PathValue("day"))
	if err != nil {
		httpx.Error(w, &domain.ValidationError{Message: "day must be an integer"})
		return
	}
	orderIdx, err := strconv.Atoi(r.PathValue("order"))
	if err != nil {
		httpx.Error(w, &domain.ValidationError{Message: "order must be an integer"})
		return
	}
	// Ownership check — load the plan, confirm user.
	p, err := h.svc.Get(r.Context(), planID)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	if p.UserID != u.ID {
		httpx.Error(w, domain.ErrNotFound)
		return
	}
	alts, err := h.svc.Alternatives(r.Context(), planID, dayIdx, orderIdx, 5)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"alternatives": alts})
}

func (h *Handler) swap(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	planID := r.PathValue("id")
	dayIdx, err := strconv.Atoi(r.PathValue("day"))
	if err != nil {
		httpx.Error(w, &domain.ValidationError{Message: "day must be an integer"})
		return
	}
	orderIdx, err := strconv.Atoi(r.PathValue("order"))
	if err != nil {
		httpx.Error(w, &domain.ValidationError{Message: "order must be an integer"})
		return
	}
	var req struct {
		ExerciseID string `json:"exercise_id"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	if req.ExerciseID == "" {
		httpx.Error(w, &domain.ValidationError{Message: "exercise_id is required"})
		return
	}
	// Ownership check.
	p, err := h.svc.Get(r.Context(), planID)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	if p.UserID != u.ID {
		httpx.Error(w, domain.ErrNotFound)
		return
	}
	out, err := h.svc.SwapExercise(r.Context(), planID, dayIdx, orderIdx, req.ExerciseID)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

// generate runs the engine for the requesting user and persists the plan. An
// optional ?seed= makes the result reproducible; otherwise we vary by
// wall-clock time.
func (h *Handler) generate(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.Error(w, &domain.ValidationError{Message: "not authenticated"})
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

	out, err := h.svc.Create(r.Context(), u.ID, req, seed, recoveryAware)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, out)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	// Authorization: only the owner can read a plan. We fetch first, then
	// compare — a 404 for "not yours" would leak whether the id exists.
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
	httpx.JSON(w, http.StatusOK, map[string]any{"plans": out})
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	// Same ownership check as Get — load, verify owner, then delete.
	id := r.PathValue("id")
	p, err := h.svc.Get(r.Context(), id)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	if p.UserID != u.ID {
		httpx.Error(w, domain.ErrNotFound)
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusNoContent, nil)
}
