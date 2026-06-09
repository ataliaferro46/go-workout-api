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

// checkWeekdayRecovery returns warnings when the calendar-bound plan would
// hit the same muscle group within minRecovery days of itself. Returns nil
// when weekdays is empty (the plan isn't calendar-bound) or when every
// muscle group has at least minRecovery days between hits.
//
// Distance between two weekdays is calculated as the minimum of the forward
// and backward gaps modulo 7, so Mon→Sat counts as 2 days (not 5).
func checkWeekdayRecovery(weekdays []string, days []dayTemplate, minRecovery int) []string {
	if len(weekdays) == 0 || minRecovery <= 0 {
		return nil
	}
	// Index muscle group → list of (weekday, dayName) pairs hitting it.
	type hit struct {
		dayIdx  int    // 0..6
		dayName string // "Push", "Upper", etc.
	}
	byMuscle := make(map[domain.MuscleGroup][]hit)
	for i, tmpl := range days {
		if i >= len(weekdays) {
			break
		}
		idx, ok := domain.ValidWeekdays[weekdays[i]]
		if !ok {
			continue
		}
		for _, m := range tmpl.Muscles {
			byMuscle[m] = append(byMuscle[m], hit{dayIdx: idx, dayName: tmpl.Name})
		}
	}
	seen := make(map[string]bool)
	var warnings []string
	for muscle, hits := range byMuscle {
		for a := 0; a < len(hits); a++ {
			for b := a + 1; b < len(hits); b++ {
				diff := abs(hits[a].dayIdx - hits[b].dayIdx)
				dist := diff
				if 7-diff < dist {
					dist = 7 - diff
				}
				if dist < minRecovery {
					key := fmt.Sprintf("%s|%s|%s", muscle, hits[a].dayName, hits[b].dayName)
					if seen[key] {
						continue
					}
					seen[key] = true
					warnings = append(warnings, fmt.Sprintf(
						"%s hit by %s and %s with only %d day(s) between — below your %d-day recovery window",
						muscle, hits[a].dayName, hits[b].dayName, dist, minRecovery,
					))
				}
			}
		}
	}
	return warnings
}

// helpfulSplitMismatch maps a (split, days) combination we know doesn't work
// to an error message that names the right alternative. Falls back to a
// generic message for unmapped combinations.
func helpfulSplitMismatch(split string, days int) string {
	switch split {
	case "ppl":
		if days == 2 {
			return "PPL needs at least 3 days. For 2 days, use 'Full Body' so every muscle group gets work."
		}
		return fmt.Sprintf("PPL needs at least 3 days/week; got %d.", days)
	case "ppl_upper_lower":
		return fmt.Sprintf("PPL + UL is specifically a 5-day program; %d days doesn't fit. "+
			"Try 'Upper / Lower' for 4 days or 'PPL' for 3 or more.", days)
	}
	return fmt.Sprintf("split '%s' does not support %d days/week", split, days)
}

// imbalanceWarning describes a known per-muscle-group imbalance the user
// implicitly accepted by picking a split + day count that doesn't divide
// cleanly. Returned as a plan-level warning (not an error) so the plan
// still generates — the imbalance is sometimes exactly what the user
// wants (e.g. extra upper-body emphasis with PPL at 5 days).
func imbalanceWarning(split string, days []dayTemplate) string {
	if split != "ppl" || len(days)%3 == 0 {
		return ""
	}
	pushN, pullN, legsN := 0, 0, 0
	for _, d := range days {
		switch d.Name {
		case "Push":
			pushN++
		case "Pull":
			pullN++
		case "Legs":
			legsN++
		}
	}
	return fmt.Sprintf(
		"PPL truncated at %d days — Push %d, Pull %d, Legs %d. The cycle "+
			"resets each Sunday; if you want even frequency across all groups, "+
			"use 6 days or switch to 'PPL + UL'.",
		len(days), pushN, pullN, legsN,
	)
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
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

	// Choose the split. "auto" (or empty) falls back to the days-per-week
	// driven default; any other value uses splitByName, which refuses
	// nonsensical combinations like ppl-at-4-days. We surface a *helpful*
	// error pointing at the canonical alternative for known mismatches —
	// the typical PPL-at-5 case is exactly what `ppl_upper_lower` covers.
	var split splitPlan
	if req.Split == "" || req.Split == "auto" {
		split = chooseSplit(req.DaysPerWeek)
	} else {
		named := splitByName(req.Split, req.DaysPerWeek)
		if named == nil {
			return domain.WorkoutPlan{}, &domain.ValidationError{
				Message: helpfulSplitMismatch(req.Split, req.DaysPerWeek),
			}
		}
		split = *named
	}
	count := exerciseCount(req.SessionMinutesOrDefault(), req.Experience)
	recoveryAdjustment, recoveryWarning := recoveryAdjustmentFor(req.RecoveryHint)
	recoveryWarnings := checkWeekdayRecovery(req.Weekdays, split.Days, req.MinRecoveryDaysOrDefault())
	splitWarning := imbalanceWarning(req.Split, split.Days)

	used := make(map[string]int)
	usedRegions := make(map[domain.MuscleGroup]map[string]int)
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
	if splitWarning != "" {
		plan.Warnings = append(plan.Warnings, splitWarning)
	}
	plan.Warnings = append(plan.Warnings, recoveryWarnings...)

	for i, tmpl := range split.Days {
		exercises := selectForDay(tmpl, pool, count, used, usedRegions, g.rng, recoveryAdjustment)

		day := domain.PlanDay{
			Index:     i + 1,
			Name:      tmpl.Name,
			Exercises: make([]domain.PlanExercise, 0, len(exercises)),
		}
		if i < len(req.Weekdays) {
			day.Weekday = req.Weekdays[i]
		}
		// Locate the index of the LAST compound in this day's selection so
		// assignSetType can emit AMRAP on it (not every compound — just
		// the heaviest closer, typically the final compound slot).
		lastCompoundIdx := -1
		for k, e := range exercises {
			if e.Compound {
				lastCompoundIdx = k
			}
		}
		for j, ex := range exercises {
			p := prescribe(req, ex)
			setType, note := assignSetType(req.Goal, ex, j == lastCompoundIdx)
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
		// Post-process: strip duplicate warmups for muscles already
		// trained earlier in the same session, ensure each main muscle
		// hits a 5-set weekly-minimum-per-session floor, then pair
		// adjacent isolation antagonists as supersets.
		applyWarmupContext(day.Exercises)
		ensureMinSetsPerMuscle(day.Exercises, tmpl.Muscles, 5)
		pairSupersets(req.Goal, day.Exercises)

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
