package plan

import (
	"context"
	"math/rand"
	"strings"
	"time"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
)

// IDGenerator produces unique identifiers. Injected so tests can use
// deterministic IDs instead of random UUIDs.
type IDGenerator func() string

// Clock returns the current time. Injected so tests are deterministic and not
// dependent on the wall clock.
type Clock func() time.Time

// LibrarySource returns the current exercise library snapshot. Plan.Service
// holds a LibrarySource (not a static slice) so admin edits to the exercise
// library take effect on the next Generate call without a process restart.
// In production, main.go passes exercise.Service.Snapshot; in tests, a
// closure over a static fixture.
type LibrarySource func() []domain.Exercise

// RecoverySource returns the user's latest recovery-shaped reading
// normalized to 0.0–1.0. Returns false if no fresh recovery is available;
// the plan service treats absence as "no adjustment." Production wires a
// biometrics.Service adapter here; tests can pass nil to disable recovery-
// awareness entirely.
type RecoverySource interface {
	LatestRecovery(ctx context.Context, userID string) (value float64, fresh bool, err error)
}

// Service owns the lifecycle of generated workout plans: it wraps the
// Generator with persistence, ID generation, and timestamping. The Handler
// depends on the Service; the Service depends on the Repository interface
// and the LibrarySource function.
type Service struct {
	library  LibrarySource
	repo     Repository
	recovery RecoverySource
	newID    IDGenerator
	now      Clock
}

// NewService constructs a Service. Passing nil for newID or now selects
// production defaults (random UUIDs and the system clock). recovery may be
// nil — recovery-aware generation is disabled in that case.
func NewService(library LibrarySource, repo Repository, recovery RecoverySource, newID IDGenerator, now Clock) *Service {
	if newID == nil {
		newID = NewUUID
	}
	if now == nil {
		now = time.Now
	}
	return &Service{library: library, repo: repo, recovery: recovery, newID: newID, now: now}
}

// Create runs the generation engine for the given user and request, stamps
// persistence metadata (ID, UserID, CreatedAt), and persists the plan via the
// repository. The seed makes generation reproducible — pass time.Now().UnixNano()
// for variety, or a fixed value for determinism. When recoveryAware is true
// and a fresh recovery reading exists, the engine biases scoring away from
// high-intensity compounds (ADR-051) and emits a warning explaining the
// adjustment.
func (s *Service) Create(ctx context.Context, userID string, req domain.GenerateRequest, seed int64, recoveryAware bool) (domain.WorkoutPlan, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return domain.WorkoutPlan{}, &domain.ValidationError{Message: "user id is required"}
	}

	// Recovery-aware path: populate req.RecoveryHint from biometrics so the
	// Generator's scoring downstream picks it up. Failure to read recovery
	// is silent — we proceed without a hint rather than failing the plan
	// generation.
	if recoveryAware && s.recovery != nil {
		if value, fresh, err := s.recovery.LatestRecovery(ctx, userID); err == nil && fresh {
			req.RecoveryHint = &value
		}
	}

	// Build a fresh Generator per call so concurrent requests do not share
	// mutable RNG state (mirrors the rationale in ADR-013). The library is
	// read at call time from the LibrarySource so admin edits take effect
	// immediately.
	gen := NewGenerator(s.library(), seed)
	p, err := gen.Generate(req)
	if err != nil {
		return domain.WorkoutPlan{}, err
	}

	p.ID = s.newID()
	p.UserID = userID
	p.CreatedAt = s.now().UTC()
	if p.Warnings == nil {
		p.Warnings = []string{}
	}

	if err := s.repo.Create(ctx, p); err != nil {
		return domain.WorkoutPlan{}, err
	}
	return p, nil
}

// Get returns the persisted plan with the given ID.
func (s *Service) Get(ctx context.Context, id string) (domain.WorkoutPlan, error) {
	return s.repo.Get(ctx, id)
}

// QuickDayRequest captures the input for an ad-hoc single-day workout
// build — the user picks specific muscle groups they want to train today
// rather than generating a full weekly plan. This is the "I just want a
// chest+triceps session right now" path.
type QuickDayRequest struct {
	Goal           domain.Goal
	Experience     domain.ExperienceLevel
	Equipment      []domain.Equipment
	Injuries       []domain.BodyPart
	Muscles        []domain.MuscleGroup
	SessionMinutes int
	SetsOverride   *int
	RecoveryHint   *float64
}

// BuildAdHocDay generates a single training day targeted at the chosen
// muscle groups. Reuses the same selection algorithm (per-pattern compound
// dedupe, per-muscle isolation dedupe, region rotation, warmup context,
// set-type variance) the weekly generator runs, but with a custom template
// derived from the user's muscle picks.
//
// Returns the day's PlanExercises (not a persisted plan). The caller can
// hand them to the workout package to create an immediately-loggable
// session via POST /v1/workouts.
func (s *Service) BuildAdHocDay(ctx context.Context, req QuickDayRequest) (domain.PlanDay, error) {
	if len(req.Muscles) == 0 {
		return domain.PlanDay{}, &domain.ValidationError{Message: "pick at least one muscle group"}
	}
	// Build a GenerateRequest skeleton so we can reuse the candidate pool
	// + scoring + warmup logic with no surprise.
	gen := domain.GenerateRequest{
		Goal:               req.Goal,
		Experience:         req.Experience,
		DaysPerWeek:        2, // bypasses the days_per_week>=2 validation; not used by our path
		SessionMinutes:     req.SessionMinutes,
		AvailableEquipment: req.Equipment,
		Injuries:           req.Injuries,
		SetsOverride:       req.SetsOverride,
		RecoveryHint:       req.RecoveryHint,
	}
	if err := gen.Validate(); err != nil {
		return domain.PlanDay{}, err
	}
	pool := buildCandidatePool(gen, s.library())
	if len(pool) == 0 {
		return domain.PlanDay{}, &domain.ValidationError{
			Message: "no exercises match the given equipment, experience, and injury constraints",
		}
	}

	tmpl := buildAdHocTemplate(req.Muscles)
	count := exerciseCount(gen.SessionMinutesOrDefault(), gen.Experience)
	if count > len(tmpl.Slots) {
		count = len(tmpl.Slots)
	}

	used := make(map[string]int)
	usedRegions := make(map[domain.MuscleGroup]map[string]int)
	rng := newSeededRNG()
	recoveryAdj, _ := recoveryAdjustmentFor(req.RecoveryHint)
	picked := selectForDay(tmpl, pool, count, used, usedRegions, rng, recoveryAdj)

	// Materialize the PlanDay with full prescriptions, warmup context, and
	// set-type variance (same path the weekly generator uses).
	day := domain.PlanDay{
		Index:     1,
		Name:      muscleSummary(req.Muscles),
		Exercises: make([]domain.PlanExercise, 0, len(picked)),
	}
	lastCompoundIdx := -1
	for k, e := range picked {
		if e.Compound {
			lastCompoundIdx = k
		}
	}
	for j, ex := range picked {
		p := prescribe(gen, ex)
		setType, note := assignSetType(gen.Goal, ex, j == lastCompoundIdx)
		day.Exercises = append(day.Exercises, domain.PlanExercise{
			Exercise:    ex,
			Order:       j + 1,
			Warmups:     p.warmups,
			Sets:        p.sets,
			RepsLow:     p.repsLow,
			RepsHigh:    p.repsHigh,
			RestSeconds: p.restSeconds,
			SetType:     setType,
			SetTypeNote: note,
		})
	}
	applyWarmupContext(day.Exercises)
	ensureMinSetsPerMuscle(day.Exercises, req.Muscles, 5)
	pairSupersets(gen.Goal, day.Exercises)
	return day, nil
}

// buildAdHocTemplate maps a user's chosen muscle groups to an ordered list
// of slots that hit each group with sensible programming — one compound
// (where applicable) + targeted isolations per muscle. Muscles like Biceps
// / Triceps / Calves / Core get pure isolation slots since they don't have
// their own compound patterns in the library.
func buildAdHocTemplate(muscles []domain.MuscleGroup) dayTemplate {
	slots := make([]slot, 0, len(muscles)*2)
	for _, m := range muscles {
		switch m {
		case domain.Chest:
			slots = append(slots,
				slot{Pattern: domain.HorizontalPush, Muscle: domain.Chest},
				slot{Pattern: domain.Isolation, Muscle: domain.Chest})
		case domain.Back:
			slots = append(slots,
				slot{Pattern: domain.VerticalPull, Muscle: domain.Back},
				slot{Pattern: domain.HorizontalPull, Muscle: domain.Back},
				slot{Pattern: domain.Isolation, Muscle: domain.Back})
		case domain.Shoulders:
			slots = append(slots,
				slot{Pattern: domain.VerticalPush, Muscle: domain.Shoulders},
				slot{Pattern: domain.Isolation, Muscle: domain.Shoulders})
		case domain.Quads:
			slots = append(slots,
				slot{Pattern: domain.SquatPattern, Muscle: domain.Quads},
				slot{Pattern: domain.LungePattern, Muscle: domain.Quads},
				slot{Pattern: domain.Isolation, Muscle: domain.Quads})
		case domain.Hamstrings:
			slots = append(slots,
				slot{Pattern: domain.HingePattern, Muscle: domain.Hamstrings},
				slot{Pattern: domain.Isolation, Muscle: domain.Hamstrings})
		case domain.Glutes:
			slots = append(slots,
				slot{Pattern: domain.HingePattern, Muscle: domain.Glutes},
				slot{Pattern: domain.LungePattern, Muscle: domain.Glutes})
		case domain.Biceps:
			slots = append(slots,
				slot{Pattern: domain.Isolation, Muscle: domain.Biceps},
				slot{Pattern: domain.Isolation, Muscle: domain.Biceps})
		case domain.Triceps:
			slots = append(slots,
				slot{Pattern: domain.Isolation, Muscle: domain.Triceps},
				slot{Pattern: domain.Isolation, Muscle: domain.Triceps})
		case domain.Calves:
			slots = append(slots,
				slot{Pattern: domain.Isolation, Muscle: domain.Calves},
				slot{Pattern: domain.Isolation, Muscle: domain.Calves})
		case domain.Core:
			slots = append(slots,
				slot{Pattern: domain.CorePattern, Muscle: domain.Core},
				slot{Pattern: domain.CorePattern, Muscle: domain.Core})
		}
	}
	return dayTemplate{Name: muscleSummary(muscles), Slots: slots, Muscles: muscles}
}

// muscleSummary formats the muscle list into a short title-case label.
// ["chest", "triceps"] -> "Chest + Triceps".
func muscleSummary(muscles []domain.MuscleGroup) string {
	parts := make([]string, 0, len(muscles))
	for _, m := range muscles {
		s := string(m)
		if len(s) > 0 {
			parts = append(parts, strings.ToUpper(s[:1])+s[1:])
		}
	}
	if len(parts) == 0 {
		return "Custom Workout"
	}
	return strings.Join(parts, " + ")
}

func newSeededRNG() *rand.Rand {
	// Same approach the handler takes — wall-clock seeded so each call
	// varies. Deterministic seeds are only useful for tests.
	return rand.New(rand.NewSource(time.Now().UnixNano()))
}

// ListByUser returns all persisted plans for the given user.
func (s *Service) ListByUser(ctx context.Context, userID string) ([]domain.WorkoutPlan, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, &domain.ValidationError{Message: "user id is required"}
	}
	return s.repo.ListByUser(ctx, userID)
}

// Delete removes a persisted plan by ID.
func (s *Service) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

// LatestRecoveryHint pulls the user's freshest recovery reading. Returns
// nil when no source is wired or the reading isn't fresh. Used by the
// quick-day endpoint so an ad-hoc workout can opt into recovery-aware
// programming the same way the weekly generator does.
func (s *Service) LatestRecoveryHint(ctx context.Context, userID string) *float64 {
	if s.recovery == nil {
		return nil
	}
	value, fresh, err := s.recovery.LatestRecovery(ctx, userID)
	if err != nil || !fresh {
		return nil
	}
	return &value
}

// ReorderDays accepts a new day order as a slice of the existing day_idx
// values in their desired sequence and remaps them to 1..N. So passing
// [2, 1, 3, 4, 5] for a 5-day plan swaps days 1 and 2 (the user's "do
// pull before push today" case). Returns the updated plan.
func (s *Service) ReorderDays(ctx context.Context, planID string, order []int) (domain.WorkoutPlan, error) {
	p, err := s.repo.Get(ctx, planID)
	if err != nil {
		return domain.WorkoutPlan{}, err
	}
	if len(order) != len(p.Days) {
		return domain.WorkoutPlan{}, &domain.ValidationError{
			Message: "order length must equal day count",
		}
	}
	// Validate: order is a permutation of the existing day indices.
	existing := make(map[int]bool, len(p.Days))
	for _, d := range p.Days {
		existing[d.Index] = true
	}
	seen := make(map[int]bool, len(order))
	for _, idx := range order {
		if !existing[idx] {
			return domain.WorkoutPlan{}, &domain.ValidationError{
				Message: "unknown day in order",
			}
		}
		if seen[idx] {
			return domain.WorkoutPlan{}, &domain.ValidationError{
				Message: "duplicate day in order",
			}
		}
		seen[idx] = true
	}
	// Build old→new mapping: order[i] (old idx) → i+1 (new idx).
	mapping := make(map[int]int, len(order))
	for i, oldIdx := range order {
		mapping[oldIdx] = i + 1
	}
	if err := s.repo.ReorderDays(ctx, planID, mapping); err != nil {
		return domain.WorkoutPlan{}, err
	}
	return s.repo.Get(ctx, planID)
}

// Alternatives returns up to `max` exercises the user could swap in for
// the exercise at (dayIdx, orderIdx). Matches are filtered to the same
// primary muscle and (for compounds) the same movement pattern so a swap
// preserves the slot's training intent — the user keeps "horizontal push,
// chest" and just picks a different vehicle (Dumbbell Bench → Machine
// Chest Press → Push-Up). Same equipment / injury filters apply.
func (s *Service) Alternatives(ctx context.Context, planID string, dayIdx, orderIdx int, max int) ([]domain.Exercise, error) {
	if max <= 0 {
		max = 5
	}
	p, err := s.repo.Get(ctx, planID)
	if err != nil {
		return nil, err
	}
	target, ok := findExerciseInPlan(p, dayIdx, orderIdx)
	if !ok {
		return nil, domain.ErrNotFound
	}
	out := make([]domain.Exercise, 0, max)
	for _, candidate := range s.library() {
		if candidate.ID == target.ID {
			continue
		}
		if candidate.PrimaryMuscle != target.PrimaryMuscle {
			continue
		}
		// For compounds, also require the same movement pattern — a bench
		// press alternative shouldn't be a fly. For isolations, allow any
		// pattern as long as the muscle matches.
		if target.Compound && candidate.Pattern != target.Pattern {
			continue
		}
		out = append(out, candidate)
		if len(out) >= max*3 {
			break // gather generously, we'll rank+trim
		}
	}
	// Rank: same pattern wins over different pattern; same region preferred;
	// same compound flag preferred. (Variance is fine for the user's
	// choice; the order should help the obvious candidate float up.)
	rankAlternatives(out, target)
	if len(out) > max {
		out = out[:max]
	}
	return out, nil
}

// SwapExercise replaces the exercise at (dayIdx, orderIdx) with the one
// identified by newExerciseID. Sets / reps / rest are preserved; warmups
// are recomputed for the new exercise (compound vs isolation, bodyweight
// vs loaded) and then context-trimmed against the rest of the day so a
// second exercise hitting an already-warmed muscle doesn't carry a
// redundant ramp. Returns the updated plan.
func (s *Service) SwapExercise(ctx context.Context, planID string, dayIdx, orderIdx int, newExerciseID string) (domain.WorkoutPlan, error) {
	p, err := s.repo.Get(ctx, planID)
	if err != nil {
		return domain.WorkoutPlan{}, err
	}
	var newEx domain.Exercise
	found := false
	for _, e := range s.library() {
		if e.ID == newExerciseID {
			newEx = e
			found = true
			break
		}
	}
	if !found {
		return domain.WorkoutPlan{}, &domain.ValidationError{Message: "exercise not in library: " + newExerciseID}
	}
	// Recompute warmups for the new exercise based on goal + load type.
	warmups := warmupSetsFor(p.Goal, newEx)

	// Replay the day's exercises with the swap applied, run the session
	// warmup-context pass, then persist whatever warmups the swapped
	// exercise ends up with after context trimming.
	for _, d := range p.Days {
		if d.Index != dayIdx {
			continue
		}
		// Materialize a candidate day with the swap applied so we can run
		// applyWarmupContext over it.
		candidate := make([]domain.PlanExercise, len(d.Exercises))
		copy(candidate, d.Exercises)
		for i := range candidate {
			if candidate[i].Order == orderIdx {
				candidate[i].Exercise = newEx
				candidate[i].Warmups = warmups
			}
		}
		applyWarmupContext(candidate)
		for _, pe := range candidate {
			if pe.Order == orderIdx {
				warmups = pe.Warmups
				break
			}
		}
	}

	if err := s.repo.UpdateExercise(ctx, planID, dayIdx, orderIdx, newEx, warmups); err != nil {
		return domain.WorkoutPlan{}, err
	}
	return s.repo.Get(ctx, planID)
}

// findExerciseInPlan locates the prescribed exercise at (dayIdx, orderIdx)
// for use by alternative-lookup, returning the underlying library Exercise
// (not the PlanExercise wrapper).
func findExerciseInPlan(p domain.WorkoutPlan, dayIdx, orderIdx int) (domain.Exercise, bool) {
	for _, d := range p.Days {
		if d.Index != dayIdx {
			continue
		}
		for _, pe := range d.Exercises {
			if pe.Order == orderIdx {
				return pe.Exercise, true
			}
		}
	}
	return domain.Exercise{}, false
}

// rankAlternatives sorts candidates so the most-direct swap appears first:
// same pattern > different pattern; same region > different > unset; same
// compound flag > different.
func rankAlternatives(cands []domain.Exercise, target domain.Exercise) {
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
	// Simple insertion sort — N is tiny (<= 20 typical).
	for i := 1; i < len(cands); i++ {
		j := i
		for j > 0 && score(cands[j]) > score(cands[j-1]) {
			cands[j], cands[j-1] = cands[j-1], cands[j]
			j--
		}
	}
}
