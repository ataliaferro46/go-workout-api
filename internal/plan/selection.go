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

// selectForDay walks the template's ordered slots and picks one exercise per
// slot. The pattern + muscle hint on each slot is a hard preference (we
// prefer compounds that match both, then degrade) but unmatched slots are
// skipped rather than substituted with random fill — that's the rule the
// old algorithm broke by chasing "best of any pattern" and inadvertently
// stacking compounds on the same muscle group.
//
// Constraints enforced inside this function:
//
//   - At most one compound per movement Pattern per session. Prevents
//     Bench Press + Incline Bench Press in the same Push day (same
//     pattern → same primary muscle stimulus).
//   - At most one isolation per primary muscle per session. Prevents
//     three lateral raises stacking on a single Push day.
//   - count caps the total number of exercises (templates may be longer
//     than count when a user picks a short session, in which case the
//     trailing isolation slots are dropped).
//
// `used` is the cross-plan usage counter — repeated picks of the same
// exercise across the week are softly penalized so later days favor
// variety. `recoveryAdjustment` is subtracted from compound scores per
// ADR-051.
func selectForDay(tmpl dayTemplate, pool []domain.Exercise, count int, used map[string]int, usedRegions map[domain.MuscleGroup]map[string]int, rng *rand.Rand, recoveryAdjustment float64) []domain.Exercise {
	dayMuscles := toMuscleSet(tmpl.Muscles)
	chosen := make([]domain.Exercise, 0, len(tmpl.Slots))
	chosenIDs := map[string]bool{}
	compoundPatternUsed := map[domain.MovementPattern]bool{}
	isolationMuscleUsed := map[domain.MuscleGroup]bool{}

	record := func(ex domain.Exercise) {
		chosenIDs[ex.ID] = true
		used[ex.ID]++
		if ex.Region != "" {
			if usedRegions[ex.PrimaryMuscle] == nil {
				usedRegions[ex.PrimaryMuscle] = map[string]int{}
			}
			usedRegions[ex.PrimaryMuscle][ex.Region]++
		}
		if ex.Compound {
			compoundPatternUsed[ex.Pattern] = true
		} else {
			isolationMuscleUsed[ex.PrimaryMuscle] = true
		}
	}

	for _, s := range tmpl.Slots {
		if len(chosen) >= count {
			break
		}
		ex, ok := bestMatchForSlot(pool, s, dayMuscles, chosenIDs, used, usedRegions,
			compoundPatternUsed, isolationMuscleUsed, rng, recoveryAdjustment)
		if !ok {
			continue
		}
		chosen = append(chosen, ex)
		record(ex)
	}

	for len(chosen) < count {
		s := slot{Pattern: domain.Isolation}
		ex, ok := bestMatchForSlot(pool, s, dayMuscles, chosenIDs, used, usedRegions,
			compoundPatternUsed, isolationMuscleUsed, rng, recoveryAdjustment)
		if !ok {
			break
		}
		chosen = append(chosen, ex)
		record(ex)
	}
	return chosen
}

// bestMatchForSlot returns the highest-scoring exercise that satisfies the
// slot's pattern requirement and respects the session-level dedupe rules.
// Returns ok=false when no eligible candidate exists (e.g., user has no
// equipment for any matching exercise — the day will simply have one
// fewer movement, surfaced as a plan warning by the generator).
func bestMatchForSlot(
	pool []domain.Exercise,
	s slot,
	dayMuscles map[domain.MuscleGroup]bool,
	chosenIDs map[string]bool,
	used map[string]int,
	usedRegions map[domain.MuscleGroup]map[string]int,
	compoundPatternUsed map[domain.MovementPattern]bool,
	isolationMuscleUsed map[domain.MuscleGroup]bool,
	rng *rand.Rand,
	recoveryAdjustment float64,
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
		if ex.Pattern != s.Pattern {
			continue
		}
		// Compound dedupe per pattern: skip if a compound for this pattern
		// has already been placed.
		if ex.Compound && compoundPatternUsed[ex.Pattern] {
			continue
		}
		// Isolation dedupe per primary muscle: skip if we already have an
		// isolation hitting this muscle in this session.
		if !ex.Compound && isolationMuscleUsed[ex.PrimaryMuscle] {
			continue
		}
		// Strict primary-muscle gate. The old algorithm allowed any exercise
		// whose secondary muscles overlapped the day, which let pulling
		// movements (Scapular Pull-Up — primary Back, secondary Shoulders)
		// sneak into Push day via the shoulder secondary tag. Requiring the
		// primary to be a day muscle eliminates that whole class of bug.
		if !dayMuscles[ex.PrimaryMuscle] {
			continue
		}
		cands = append(cands, scored{
			ex:    ex,
			score: scoreExercise(ex, s.Muscle, dayMuscles, used, usedRegions, rng, recoveryAdjustment),
		})
	}
	if len(cands) == 0 {
		return domain.Exercise{}, false
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].score == cands[j].score {
			return cands[i].ex.ID < cands[j].ex.ID
		}
		return cands[i].score > cands[j].score
	})
	return cands[0].ex, true
}

// scoreExercise ranks a candidate for a slot. Hierarchy:
//
//   - +3 compound bonus (offset by recoveryAdjustment when fatigued)
//   - +3 exact muscle hint match (slot.Muscle == ex.PrimaryMuscle) — this
//     is the biggest factor for an isolation slot, where it means "the
//     right muscle for this slot"
//   - +1 secondary muscle hint match (slot.Muscle is in ex.SecondaryMuscles)
//   - +2 primary muscle in day muscles (general fit)
//   - -1.5 per prior use across the plan (variety)
//   - +0..0.5 seeded jitter (so equally-scored candidates rotate plan to plan)
func scoreExercise(
	ex domain.Exercise,
	slotMuscle domain.MuscleGroup,
	dayMuscles map[domain.MuscleGroup]bool,
	used map[string]int,
	usedRegions map[domain.MuscleGroup]map[string]int,
	rng *rand.Rand,
	recoveryAdjustment float64,
) float64 {
	score := 0.0
	if ex.Compound {
		score += 3.0 - recoveryAdjustment
	}
	if slotMuscle != "" {
		if ex.PrimaryMuscle == slotMuscle {
			score += 3.0
		} else if hasMuscle(ex.SecondaryMuscles, slotMuscle) {
			score += 1.0
		}
	}
	if dayMuscles[ex.PrimaryMuscle] {
		score += 2.0
	}
	score -= float64(used[ex.ID]) * 1.5
	// Region rotation: penalize regions of this primary muscle that have
	// already been hit elsewhere in the plan. This is what makes the
	// engine pick "Cable Overhead Triceps Extension" (long head) on
	// Tuesday's Push after "Cable Triceps Pushdown" (lateral head) ran on
	// Monday's Push. Untagged exercises (Region == "") receive no
	// adjustment.
	if ex.Region != "" {
		if hits := usedRegions[ex.PrimaryMuscle][ex.Region]; hits > 0 {
			score -= float64(hits) * 1.25
		}
	}
	score += rng.Float64() * 0.5
	return score
}

func hasMuscle(ms []domain.MuscleGroup, target domain.MuscleGroup) bool {
	for _, m := range ms {
		if m == target {
			return true
		}
	}
	return false
}

func toMuscleSet(ms []domain.MuscleGroup) map[domain.MuscleGroup]bool {
	set := make(map[domain.MuscleGroup]bool, len(ms))
	for _, m := range ms {
		set[m] = true
	}
	return set
}
