// Package plan contains the workout generation engine and its HTTP handler.
// Given a validated request it chooses a training split, filters the exercise
// library by the user's equipment / experience / injuries, selects movements
// for balanced muscle coverage, and prescribes sets, reps, and rest.
package plan

import (
	"fmt"
	"math/rand"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
)

// Generator builds workout plans. It holds the exercise library and a source
// of randomness; injecting the RNG makes generation reproducible in tests
// while allowing variety in production.
type Generator struct {
	library []domain.Exercise
	rng     *rand.Rand
}

// NewGenerator returns a Generator over the given library, seeded with seed. A
// freshly constructed Generator produces the same plan for the same request
// and seed.
func NewGenerator(library []domain.Exercise, seed int64) *Generator {
	return &Generator{
		library: library,
		rng:     rand.New(rand.NewSource(seed)),
	}
}

// recoveryAdjustmentFor maps a normalized recovery score (0.0–1.0) into a
// compound-score penalty per ADR-051. Fully recovered (≥0.66) means no
// adjustment; medium (0.33–0.66) shaves the compound bonus; low (<0.33)
// shaves it harder, biasing selection toward easier variants. Returns the
// penalty plus a human-readable warning the caller can attach to the plan.
func recoveryAdjustmentFor(recovery *float64) (float64, string) {
	if recovery == nil {
		return 0, ""
	}
	pct := int(*recovery*100 + 0.5)
	switch {
	case *recovery < 0.33:
		return 1.5, fmt.Sprintf("recovery low (%d%%); biased toward easier variants", pct)
	case *recovery < 0.66:
		return 0.5, fmt.Sprintf("recovery medium (%d%%); biased toward easier variants", pct)
	default:
		return 0, ""
	}
}

// Generate validates the request and returns a workout plan, or a
// *domain.ValidationError if the request is invalid or no exercises fit the
// constraints.
func (g *Generator) Generate(req domain.GenerateRequest) (domain.WorkoutPlan, error) {
	if err := req.Validate(); err != nil {
		return domain.WorkoutPlan{}, err
	}

	pool := buildCandidatePool(req, g.library)
	if len(pool) == 0 {
		return domain.WorkoutPlan{}, &domain.ValidationError{
			Message: "no exercises match the given equipment, experience, and injury constraints",
		}
	}

	split := chooseSplit(req.DaysPerWeek)
	count := exerciseCount(req.SessionMinutesOrDefault(), req.Experience)
	recoveryAdjustment, recoveryWarning := recoveryAdjustmentFor(req.RecoveryHint)

	used := make(map[string]int)
	plan := domain.WorkoutPlan{
		Goal:        req.Goal,
		Experience:  req.Experience,
		DaysPerWeek: req.DaysPerWeek,
		Split:       split.Name,
		Days:        make([]domain.PlanDay, 0, len(split.Days)),
	}
	if recoveryWarning != "" {
		plan.Warnings = append(plan.Warnings, recoveryWarning)
	}

	for i, tmpl := range split.Days {
		exercises := selectForDay(tmpl, pool, count, used, g.rng, recoveryAdjustment)

		day := domain.PlanDay{
			Index:     i + 1,
			Name:      tmpl.Name,
			Exercises: make([]domain.PlanExercise, 0, len(exercises)),
		}
		for j, ex := range exercises {
			p := prescribe(req, ex)
			day.Exercises = append(day.Exercises, domain.PlanExercise{
				Exercise:    ex,
				Order:       j + 1,
				Sets:        p.sets,
				RepsLow:     p.repsLow,
				RepsHigh:    p.repsHigh,
				RestSeconds: p.restSeconds,
			})
		}

		if len(day.Exercises) < count {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf(
				"%s day has %d of %d target exercises; equipment or injury constraints limited the selection",
				tmpl.Name, len(day.Exercises), count,
			))
		}
		plan.Days = append(plan.Days, day)
	}

	return plan, nil
}
