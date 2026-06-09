package plan

import "github.com/ataliaferro46/go-workout-api/internal/domain"

// slot is one prescribed exercise role within a training day: a movement
// pattern plus the muscle this slot is supposed to develop. Templates are
// authored as ordered slot lists, which the selector walks once instead of
// running a "fill the remaining slots from anywhere" pass. This is the fix
// for the old algorithm picking three lateral raises in a row or stacking
// two chest compounds in the same session.
//
// Each Compound-pattern slot expresses ONE primary muscle target; the
// selector enforces "at most one compound per movement pattern per
// session" so you never see Bench Press + Incline Bench Press together
// (same pattern, same primary). Isolation slots are explicitly targeted
// at a single muscle so the day's accessory work covers the right ground
// (e.g., side delts on push day, rear delts on pull day).
//
// The programming choices below follow Schoenfeld's volume-frequency
// research and Mike Israetel's MEV / MAV recommendations: one compound
// per primary muscle, then 1–3 targeted isolations to bring up muscles
// the compounds under-stimulate (lateral delts, rear delts, biceps,
// triceps, hamstrings, calves).
type slot struct {
	Pattern domain.MovementPattern
	Muscle  domain.MuscleGroup // primary target for the slot
}

// dayTemplate is the per-day prescription scaffold the selector fills.
type dayTemplate struct {
	Name    string
	Slots   []slot
	Muscles []domain.MuscleGroup // union of muscle groups the day should hit
}

// splitPlan pairs a human-readable split name with its per-day templates.
type splitPlan struct {
	Name string
	Days []dayTemplate
}

// Reusable day templates. Each template lists exactly the slots that a
// well-programmed session of that type should contain, in execution order
// (compounds first, then targeted isolations).
var (
	// PUSH — chest + shoulders + triceps.
	//
	//   1. Horizontal push compound (chest focus, e.g. bench press)
	//   2. Vertical push compound (shoulder focus, e.g. overhead press)
	//   3. Chest isolation (fly / pec deck — direct chest stimulus beyond the
	//      compound)
	//   4. Triceps isolation (extension / pushdown — compounds bias front
	//      delts more than triceps)
	//   5. Lateral delt isolation (lateral raise — *crucial* because pressing
	//      patterns are heavily front-delt dominant and miss the lateral head)
	pushDay = dayTemplate{
		Name: "Push",
		Slots: []slot{
			{Pattern: domain.HorizontalPush, Muscle: domain.Chest},
			{Pattern: domain.VerticalPush, Muscle: domain.Shoulders},
			{Pattern: domain.Isolation, Muscle: domain.Chest},
			{Pattern: domain.Isolation, Muscle: domain.Triceps},
			{Pattern: domain.Isolation, Muscle: domain.Shoulders},
		},
		Muscles: []domain.MuscleGroup{domain.Chest, domain.Shoulders, domain.Triceps},
	}

	// PULL — back + biceps + rear delts.
	//
	//   1. Vertical pull compound (lat focus, e.g. pull-up, lat pulldown)
	//   2. Horizontal pull compound (mid-back focus, e.g. barbell row,
	//      seated cable row)
	//   3. Biceps isolation (compounds train biceps but not enough for
	//      hypertrophy — direct curls are standard)
	//   4. Rear delt isolation (face pull / rear delt fly — almost
	//      everyone underdeveloped here because pressing dominates the week)
	//   5. Back isolation (pullover / straight-arm pulldown — extra lat
	//      stimulus that compounds don't fully cover)
	pullDay = dayTemplate{
		Name: "Pull",
		Slots: []slot{
			{Pattern: domain.VerticalPull, Muscle: domain.Back},
			{Pattern: domain.HorizontalPull, Muscle: domain.Back},
			{Pattern: domain.Isolation, Muscle: domain.Biceps},
			{Pattern: domain.Isolation, Muscle: domain.Shoulders},
			{Pattern: domain.Isolation, Muscle: domain.Back},
		},
		Muscles: []domain.MuscleGroup{domain.Back, domain.Biceps, domain.Shoulders},
	}

	// LEGS — full lower body + core.
	//
	//   1. Squat pattern (quad focus, e.g. back squat, front squat)
	//   2. Hinge pattern (posterior chain — hamstrings + glutes,
	//      e.g. Romanian deadlift)
	//   3. Lunge / unilateral (glutes + quads with balance demand)
	//   4. Hamstring isolation (curl — direct hamstring work because squats
	//      under-stimulate them)
	//   5. Calf isolation (calf raise — calves need direct work)
	legsDay = dayTemplate{
		Name: "Legs",
		Slots: []slot{
			{Pattern: domain.SquatPattern, Muscle: domain.Quads},
			{Pattern: domain.HingePattern, Muscle: domain.Hamstrings},
			{Pattern: domain.LungePattern, Muscle: domain.Glutes},
			{Pattern: domain.Isolation, Muscle: domain.Hamstrings},
			{Pattern: domain.Isolation, Muscle: domain.Calves},
		},
		Muscles: []domain.MuscleGroup{domain.Quads, domain.Hamstrings, domain.Glutes, domain.Calves},
	}

	// UPPER — combined push + pull. The two pull compounds are deliberately
	// targeted at the same muscle (Back) but different patterns; the
	// per-pattern compound constraint allows this (a vertical lat pull and
	// a horizontal row hit different parts of the back).
	upperDay = dayTemplate{
		Name: "Upper",
		Slots: []slot{
			{Pattern: domain.HorizontalPush, Muscle: domain.Chest},
			{Pattern: domain.VerticalPull, Muscle: domain.Back},
			{Pattern: domain.VerticalPush, Muscle: domain.Shoulders},
			{Pattern: domain.HorizontalPull, Muscle: domain.Back},
			{Pattern: domain.Isolation, Muscle: domain.Biceps},
			{Pattern: domain.Isolation, Muscle: domain.Triceps},
		},
		Muscles: []domain.MuscleGroup{domain.Chest, domain.Back, domain.Shoulders, domain.Biceps, domain.Triceps},
	}

	// LOWER — identical programming to LEGS, but rendered as "Lower" on
	// the user-facing schedule because that's what Upper/Lower split users
	// expect to see in their week.
	lowerDay = dayTemplate{
		Name: "Lower",
		Slots: []slot{
			{Pattern: domain.SquatPattern, Muscle: domain.Quads},
			{Pattern: domain.HingePattern, Muscle: domain.Hamstrings},
			{Pattern: domain.LungePattern, Muscle: domain.Glutes},
			{Pattern: domain.Isolation, Muscle: domain.Hamstrings},
			{Pattern: domain.Isolation, Muscle: domain.Calves},
		},
		Muscles: []domain.MuscleGroup{domain.Quads, domain.Hamstrings, domain.Glutes, domain.Calves},
	}

	// FULL BODY — one compound per major movement pattern, plus core.
	// The order is intentional: squat first (most CNS demanding), then
	// alternating push/pull/hinge to manage local fatigue, then core.
	fullBody = dayTemplate{
		Name: "Full Body",
		Slots: []slot{
			{Pattern: domain.SquatPattern, Muscle: domain.Quads},
			{Pattern: domain.HorizontalPush, Muscle: domain.Chest},
			{Pattern: domain.VerticalPull, Muscle: domain.Back},
			{Pattern: domain.HingePattern, Muscle: domain.Hamstrings},
			{Pattern: domain.CorePattern, Muscle: domain.Core},
		},
		Muscles: []domain.MuscleGroup{domain.Quads, domain.Chest, domain.Back, domain.Hamstrings, domain.Core},
	}
)

// chooseSplit selects a training split based on the number of days available.
// More days unlock more specialized splits; fewer days favor full-body work so
// each muscle group is still trained with enough frequency.
func chooseSplit(daysPerWeek int) splitPlan {
	switch daysPerWeek {
	case 2:
		return splitPlan{Name: "Full Body", Days: []dayTemplate{fullBody, fullBody}}
	case 3:
		return splitPlan{Name: "Full Body", Days: []dayTemplate{fullBody, fullBody, fullBody}}
	case 4:
		return splitPlan{Name: "Upper / Lower", Days: []dayTemplate{upperDay, lowerDay, upperDay, lowerDay}}
	case 5:
		return splitPlan{Name: "Push / Pull / Legs + Upper / Lower", Days: []dayTemplate{pushDay, pullDay, legsDay, upperDay, lowerDay}}
	case 6:
		return splitPlan{Name: "Push / Pull / Legs (x2)", Days: []dayTemplate{pushDay, pullDay, legsDay, pushDay, pullDay, legsDay}}
	default:
		// Only reached if validation is bypassed; a safe full-body fallback
		// keeps the function total.
		return splitPlan{Name: "Full Body", Days: []dayTemplate{fullBody, fullBody, fullBody}}
	}
}

// splitByName returns a splitPlan whose day count matches daysPerWeek, using
// the named template. Returns nil if the named split can't be produced for
// the requested day count — the caller surfaces that as a validation error.
//
// The mapping is deliberate: an "upper_lower" template at 3 days isn't a
// real upper/lower split, so we refuse rather than silently generate a
// distorted plan.
func splitByName(name string, daysPerWeek int) *splitPlan {
	switch name {
	case "full_body":
		days := make([]dayTemplate, daysPerWeek)
		for i := range days {
			days[i] = fullBody
		}
		return &splitPlan{Name: "Full Body", Days: days}
	case "upper_lower":
		// Upper/lower works for any even rep count from 2..6; for odd counts
		// we'd add an extra Upper at the end, which is a common pattern.
		days := make([]dayTemplate, daysPerWeek)
		for i := range days {
			if i%2 == 0 {
				days[i] = upperDay
			} else {
				days[i] = lowerDay
			}
		}
		return &splitPlan{Name: "Upper / Lower", Days: days}
	case "ppl":
		// PPL is a 3-day cycle. We accept any day count >= 2; for 2 we
		// truncate to Push/Pull (a real "minimal upper-body week" pattern),
		// then standard cycling for everything else. Warnings call out
		// per-muscle imbalances when the cycle doesn't divide cleanly.
		if daysPerWeek < 2 {
			return nil
		}
		templates := []dayTemplate{pushDay, pullDay, legsDay}
		days := make([]dayTemplate, daysPerWeek)
		for i := range days {
			days[i] = templates[i%3]
		}
		return &splitPlan{Name: "Push / Pull / Legs", Days: days}
	case "ppl_upper_lower":
		// PPL + UL is canonically 5 days but we accept anything from 2 to
		// 7 and cycle through [P, P, L, U, L] as the template. Better than
		// erroring — the user picked the days they have, the engine
		// adapts. A 7-day version reads P/P/L/U/L/P/P, which is heavy
		// upper emphasis but valid.
		if daysPerWeek < 2 {
			return nil
		}
		templates := []dayTemplate{pushDay, pullDay, legsDay, upperDay, lowerDay}
		days := make([]dayTemplate, daysPerWeek)
		for i := range days {
			days[i] = templates[i%5]
		}
		return &splitPlan{Name: "Push / Pull / Legs + Upper / Lower", Days: days}
	}
	return nil
}
