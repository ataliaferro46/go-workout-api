package plan

import (
	"math/rand"
	"sort"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
)

// buildCandidatePool filters the library to exercises the user can actually
// perform: equipment they have, no injury conflicts, within their experience.
func buildCandidatePool(req domain.GenerateRequest, library []domain.Exercise) []domain.Exercise {
	equip := req.EquipmentSet()
	injuries := req.InjurySet()
	out := make([]domain.Exercise, 0, len(library))
	for _, ex := range library {
		if !ex.RequiresOnly(equip) {
			continue
		}
		if ex.ConflictsWith(injuries) {
			continue
		}
		if !req.Experience.CanPerform(ex.MinLevel) {
			continue
		}
		out = append(out, ex)
	}
	return out
}

// exerciseCount estimates how many movements fit in a session, assuming about
// ten minutes per exercise (warm-up + working sets + rest), clamped to a sane
// range and capped lower for beginners.
func exerciseCount(sessionMinutes int, exp domain.ExperienceLevel) int {
	count := sessionMinutes / 10
	if count < 3 {
		count = 3
	}
	if count > 7 {
		count = 7
	}
	if exp == domain.Beginner && count > 5 {
		count = 5
	}
	return count
}

// selectForDay chooses up to count exercises for one training day. It first
// satisfies the day's priority movement patterns in order, then fills any
// remaining slots with movements that hit the day's target muscles. The used
// map (exercise ID -> times already placed in the plan) is updated in place so
// later days favor variety.
func selectForDay(tmpl dayTemplate, pool []domain.Exercise, count int, used map[string]int, rng *rand.Rand) []domain.Exercise {
	dayMuscles := toMuscleSet(tmpl.Muscles)
	chosen := make([]domain.Exercise, 0, count)
	chosenIDs := map[string]bool{}

	// First pass: one exercise per priority pattern, in priority order.
	for _, pat := range tmpl.Patterns {
		if len(chosen) >= count {
			break
		}
		if ex, ok := bestMatch(pool, pat, dayMuscles, chosenIDs, used, rng); ok {
			chosen = append(chosen, ex)
			chosenIDs[ex.ID] = true
			used[ex.ID]++
		}
	}

	// Second pass: fill remaining slots with anything that hits the day's
	// muscles. An empty pattern means "any pattern".
	for len(chosen) < count {
		ex, ok := bestMatch(pool, "", dayMuscles, chosenIDs, used, rng)
		if !ok {
			break
		}
		chosen = append(chosen, ex)
		chosenIDs[ex.ID] = true
		used[ex.ID]++
	}
	return chosen
}

// bestMatch returns the highest-scoring exercise that matches the pattern (or
// any pattern if pat is ""), targets the day's muscles, and has not already
// been chosen for this day.
func bestMatch(
	pool []domain.Exercise,
	pat domain.MovementPattern,
	dayMuscles map[domain.MuscleGroup]bool,
	chosenIDs map[string]bool,
	used map[string]int,
	rng *rand.Rand,
) (domain.Exercise, bool) {
	type scored struct {
		ex    domain.Exercise
		score float64
	}
	cands := make([]scored, 0, len(pool))
	for _, ex := range pool {
		if chosenIDs[ex.ID] {
			continue
		}
		if pat != "" && ex.Pattern != pat {
			continue
		}
		if !ex.TargetsAny(dayMuscles) {
			continue
		}
		cands = append(cands, scored{ex: ex, score: scoreExercise(ex, dayMuscles, used, rng)})
	}
	if len(cands) == 0 {
		return domain.Exercise{}, false
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].score == cands[j].score {
			return cands[i].ex.ID < cands[j].ex.ID // stable tie-break
		}
		return cands[i].score > cands[j].score
	})
	return cands[0].ex, true
}

// scoreExercise ranks a candidate: compounds and primary-muscle matches score
// higher, repeated use across the plan is penalized for variety, and a small
// seeded jitter breaks near-ties differently per seed.
func scoreExercise(ex domain.Exercise, dayMuscles map[domain.MuscleGroup]bool, used map[string]int, rng *rand.Rand) float64 {
	score := 0.0
	if ex.Compound {
		score += 3.0
	}
	if dayMuscles[ex.PrimaryMuscle] {
		score += 2.0
	}
	score -= float64(used[ex.ID]) * 1.5
	score += rng.Float64() * 0.5
	return score
}

func toMuscleSet(ms []domain.MuscleGroup) map[domain.MuscleGroup]bool {
	set := make(map[domain.MuscleGroup]bool, len(ms))
	for _, m := range ms {
		set[m] = true
	}
	return set
}
