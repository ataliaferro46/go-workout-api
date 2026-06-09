package plan

import "github.com/ataliaferro46/go-workout-api/internal/domain"

// prescription is the dosage for an exercise: how many sets and reps, and how
// long to rest. These are the standard rep/rest ranges associated with each
// training goal.
type prescription struct {
	sets        int
	repsLow     int
	repsHigh    int
	restSeconds int
	warmups     []domain.WarmupSet
}

// base rep/rest schemes per goal, before adjusting for exercise type and
// experience. Compounds add a set; isolation drops one; beginners cap volume.
var goalScheme = map[domain.Goal]prescription{
	domain.GoalStrength:       {sets: 4, repsLow: 3, repsHigh: 5, restSeconds: 180},
	domain.GoalMuscleGain:     {sets: 3, repsLow: 8, repsHigh: 12, restSeconds: 90},
	domain.GoalFatLoss:        {sets: 3, repsLow: 12, repsHigh: 15, restSeconds: 50},
	domain.GoalEndurance:      {sets: 2, repsLow: 15, repsHigh: 20, restSeconds: 40},
	domain.GoalGeneralFitness: {sets: 3, repsLow: 8, repsHigh: 12, restSeconds: 75},
}

// prescribe returns the dosage for a given exercise under the request's goal
// and experience level. The output varies by exercise "tier":
//
//   - Heavy compound (squat, bench, OHP, row, deadlift) → lower reps,
//     more weight, longer rest. The CNS-taxing money lifts.
//   - Accessory compound (lunges, dips, pull-ups, push-ups) → mid-range
//     reps, the goal's default rest.
//   - Isolation (curl, raise, fly, pushdown, extension) → higher reps,
//     less weight, less rest. Going heavy on isolations courts injury;
//     the stimulus comes from time-under-tension instead.
//
// Within each tier we apply a small id-derived jitter to the rep range
// so two muscle-gain bench-press generations don't both come back as
// exactly "4 × 8" — one might read "4 × 5-8", another "4 × 6-9".
func prescribe(req domain.GenerateRequest, ex domain.Exercise) prescription {
	p, ok := goalScheme[req.Goal]
	if !ok {
		p = goalScheme[domain.GoalGeneralFitness]
	}

	tier := classifyTier(ex)
	applyTier(&p, req.Goal, tier)

	// Set-count adjustment by tier and experience.
	switch tier {
	case tierHeavyCompound:
		p.sets++ // heavy compounds get extra volume
	case tierIsolation:
		if p.sets > 2 {
			p.sets--
		}
	}

	// Strength on isolations doesn't make sense — nudge to hypertrophy range.
	if req.Goal == domain.GoalStrength && tier == tierIsolation {
		p.repsLow, p.repsHigh = 10, 14
		p.restSeconds = 75
	}

	switch req.Experience {
	case domain.Beginner:
		if p.sets > 3 {
			p.sets = 3
		}
	case domain.Advanced:
		if tier == tierHeavyCompound {
			p.sets++
		}
	}

	if p.sets < 2 {
		p.sets = 2
	}

	// Per-exercise jitter on the rep range so the same goal+tier doesn't
	// always produce the identical 8-12. Shifts +/-1 deterministically by
	// exercise id so a given exercise stays consistent across generations.
	switch hashOdds(ex.ID, 3) {
	case 0:
		p.repsLow = maxi(1, p.repsLow-1)
	case 2:
		p.repsHigh = p.repsHigh + 1
	}

	// Working-set override — let the user dial volume directly. We honor 1
	// (a single AMRAP / high-intensity working set) even though it bypasses
	// the floor above; if you asked for 1 set, you get 1 set.
	if req.SetsOverride != nil && *req.SetsOverride > 0 {
		p.sets = *req.SetsOverride
	}

	p.warmups = warmupSetsFor(req.Goal, ex)
	return p
}

// warmupSetsFor returns the recommended warmup ramp before the working
// sets. Heuristics:
//
//   - Compound + strength goal: 3 ramps (40% × 8, 60% × 5, 80% × 3) so the
//     CNS is primed and groove is grooved before heavy triples/singles.
//   - Compound + hypertrophy/general: 2 ramps (50% × 8, 70% × 5).
//   - Isolation: 1 ramp (50% × 8). Isolation movements don't need much.
//   - Endurance: skip warmup ramps — the first working set IS the warmup
//     at endurance loads.
//
// Percentages are expressed of the user's working weight; the front-end
// multiplies once the user enters their actual load.
// assignSetType decides whether this exercise should use a non-standard
// set scheme (AMRAP, drop set, 21s). Returns (type, human-readable note).
// Superset pairing is decided separately by pairSupersets after all of a
// day's exercises are picked — it needs cross-exercise visibility this
// per-exercise function doesn't have.
//
// The rules are conservative — we don't tag every exercise — because the
// stimulus from a special set is significant. Better to apply it where it
// clearly fits than scatter it everywhere.
func assignSetType(goal domain.Goal, ex domain.Exercise, isLastCompound bool) (domain.SetType, string) {
	// 21s: classic biceps tool. Apply specifically to direct biceps
	// isolation work (curls). 25% chance via the rng-jittered hash so
	// it's an occasional surprise, not every workout.
	if ex.PrimaryMuscle == domain.Biceps && !ex.Compound &&
		(containsAny(ex.Name, "Curl") && !containsAny(ex.Name, "Hammer", "Reverse")) {
		if hashOdds(ex.ID, 4) == 0 { // 1-in-4
			return domain.SetTwentyOnes,
				"21s: 7 bottom-half reps + 7 top-half + 7 full ROM. No rest between segments."
		}
	}

	// Drop set: best on small-muscle isolation movements that don't
	// involve a spotter and can drop weight quickly (cable / dumbbell
	// finishers). Lateral raise, pushdown, calf raise, fly, rear delt
	// fly are the textbook candidates.
	if !ex.Compound &&
		(goal == domain.GoalMuscleGain || goal == domain.GoalFatLoss || goal == domain.GoalGeneralFitness) {
		if isDropSetFriendly(ex) {
			return domain.SetDropSet,
				"Drop set on the last set: at failure, drop the weight ~30% and continue without rest."
		}
	}

	// AMRAP: last working set of the heaviest compound. Universal
	// hypertrophy/general practice — drive the final set to proximity
	// of failure for stimulus.
	if isLastCompound && ex.Compound &&
		(goal == domain.GoalMuscleGain || goal == domain.GoalGeneralFitness) {
		return domain.SetAMRAP,
			"Last set: as many reps as possible with clean form."
	}

	return domain.SetStandard, ""
}

func isDropSetFriendly(ex domain.Exercise) bool {
	// Tag by ID for the obvious candidates — gives us tight control over
	// which movements get the variation. Plus a fallback by region for
	// the ones we know fit: lateral_delt, rear_delt, gastrocnemius,
	// soleus, lats_lower.
	dropSetIDs := map[string]bool{
		"lateral-raise":           true,
		"cable-lateral-raise":     true,
		"tricep-pushdown":         true,
		"tricep-kickback":         true,
		"dumbbell-curl":           true,
		"hammer-curl":             true,
		"cable-curl":              true,
		"face-pull":               true,
		"cable-reverse-fly":       true,
		"rear-delt-fly":           true,
		"dumbbell-fly":            true,
		"incline-cable-fly":       true,
		"leg-extension":           true,
		"leg-curl":                true,
		"dumbbell-calf-raise":     true,
		"standing-calf-raise":     true,
		"leg-press-calf-raise":    true,
		"seated-calf-raise":       true,
		"cable-pullover":          true,
		"straight-arm-pulldown":   true,
	}
	return dropSetIDs[ex.ID]
}

// pairSupersets walks a day's exercises and tags consecutive isolation
// pairs whose primary muscles are antagonistic (or at least
// non-overlapping small muscles) as a superset pair. Each member of the
// pair points at the other via SupersetWith. Skipped when goal is
// strength (supersets sabotage maximum-strength sessions by limiting
// rest).
//
// We modify exercises in place. Already-tagged set types (drop set,
// AMRAP, 21s from the per-exercise pass) are preserved — supersets pair
// at the SET level, so an exercise can be both "do a drop set on the
// last set" and "superset with the next exercise."
func pairSupersets(goal domain.Goal, exercises []domain.PlanExercise) {
	if goal == domain.GoalStrength {
		return
	}
	for i := 0; i+1 < len(exercises); i++ {
		a, b := exercises[i], exercises[i+1]
		if a.Exercise.Compound || b.Exercise.Compound {
			continue
		}
		if a.SetType == domain.SetSuperset || b.SetType == domain.SetSuperset {
			continue // already paired with a neighbor
		}
		if !areAntagonist(a.Exercise.PrimaryMuscle, b.Exercise.PrimaryMuscle) {
			continue
		}
		// Tag both. We deliberately overwrite a prior drop_set / amrap on
		// these slots — supersets dominate execution order, and applying
		// both would be ambiguous.
		aOrder, bOrder := a.Order, b.Order
		exercises[i].SetType = domain.SetSuperset
		exercises[i].SupersetWith = &bOrder
		exercises[i].SetTypeNote = "Superset with " + b.Exercise.Name + ": back-to-back, no rest between."
		exercises[i+1].SetType = domain.SetSuperset
		exercises[i+1].SupersetWith = &aOrder
		exercises[i+1].SetTypeNote = "Superset with " + a.Exercise.Name + ": back-to-back, no rest between."
		i++ // skip the partner so we don't double-pair
	}
}

// areAntagonist returns true when the two muscles are textbook antagonist
// pairings — opposite movements at the same joint, or simply different
// small muscles where superset stacking makes sense.
func areAntagonist(x, y domain.MuscleGroup) bool {
	if x == y {
		return false
	}
	pairs := [][2]domain.MuscleGroup{
		{domain.Biceps, domain.Triceps},
		{domain.Chest, domain.Back},
		{domain.Quads, domain.Hamstrings},
		// Lateral delt + triceps work together on push days; pair them as
		// "different small muscle groups in the same session."
		{domain.Shoulders, domain.Triceps},
		{domain.Shoulders, domain.Biceps},
		{domain.Calves, domain.Core},
	}
	for _, p := range pairs {
		if (x == p[0] && y == p[1]) || (x == p[1] && y == p[0]) {
			return true
		}
	}
	return false
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if indexOf(s, sub) >= 0 {
			return true
		}
	}
	return false
}

// hashOdds returns a deterministic value in [0, mod) derived from s. Used
// to give per-exercise occasional variations without needing the rng
// passed in.
func hashOdds(s string, mod int) int {
	if mod <= 0 {
		return 0
	}
	h := 0
	for _, r := range s {
		h = h*31 + int(r)
	}
	if h < 0 {
		h = -h
	}
	return h % mod
}

// indexOf — tiny strings.Contains substitute so we don't import a stdlib
// package just for this. The standard library is fine; this is a stylistic
// choice keeping prescription.go free of imports beyond domain.
func indexOf(s, sub string) int {
	if len(sub) == 0 {
		return 0
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// warmupSetsFor returns the recommended warmup ramp for this exercise in
// isolation. Session-level context (other exercises in the same day that
// already warmed this muscle) is applied separately by applyWarmupContext.
//
//   - Endurance: nothing — the first working set is the warmup.
//   - Bodyweight / band exercises: nothing — "50% of working weight"
//     doesn't apply when there's no loadable weight to take a fraction of.
//     A push-up or pull-up does its own warming on the first 2 reps.
//   - Compound + strength: 3 ramps (40%×8, 60%×5, 80%×3).
//   - Compound + hypertrophy/general: 2 ramps (50%×8, 70%×5).
//   - Isolation: 1 ramp (50%×8).
func warmupSetsFor(goal domain.Goal, ex domain.Exercise) []domain.WarmupSet {
	if goal == domain.GoalEndurance {
		return nil
	}
	if !usesExternalLoad(ex) {
		return nil
	}
	if ex.Compound {
		if goal == domain.GoalStrength {
			return []domain.WarmupSet{
				{Reps: 8, PercentOfWorking: 0.40},
				{Reps: 5, PercentOfWorking: 0.60},
				{Reps: 3, PercentOfWorking: 0.80},
			}
		}
		return []domain.WarmupSet{
			{Reps: 8, PercentOfWorking: 0.50},
			{Reps: 5, PercentOfWorking: 0.70},
		}
	}
	return []domain.WarmupSet{
		{Reps: 8, PercentOfWorking: 0.50},
	}
}

// exerciseTier categorizes an exercise by training-load category. Used by
// prescribe to dial reps/sets/rest appropriately — heavy compounds want
// low reps and long rest, isolations want high reps and short rest, the
// middle ground is everything else.
type exerciseTier int

const (
	tierHeavyCompound     exerciseTier = iota // squat, bench, OHP, row, deadlift
	tierAccessoryCompound                     // dips, pull-ups, lunges, step-ups, hip thrust
	tierIsolation                             // curls, raises, flies, pushdowns, extensions, calf raises
)

// classifyTier picks the tier for an exercise based on the heuristic:
//
//   - Isolation pattern → tierIsolation.
//   - Compound + heavy pattern (squat/hinge/horizontal & vertical push/pull)
//     and uses external load → tierHeavyCompound. Bodyweight compounds
//     (push-up, pull-up, dip, inverted row) drop to tierAccessoryCompound
//     because they aren't progressively loadable in the same way.
//   - Everything else → tierAccessoryCompound (lunge/step-up/hip thrust).
func classifyTier(ex domain.Exercise) exerciseTier {
	if !ex.Compound {
		return tierIsolation
	}
	heavy := false
	switch ex.Pattern {
	case domain.SquatPattern, domain.HingePattern,
		domain.HorizontalPush, domain.VerticalPush,
		domain.HorizontalPull, domain.VerticalPull:
		heavy = true
	}
	if heavy && usesExternalLoad(ex) {
		return tierHeavyCompound
	}
	return tierAccessoryCompound
}

// applyTier replaces the rep range and rest seconds on p with values
// tuned to the exercise's tier and the user's goal. This is the heart of
// the "no more 3×10 for every exercise" fix — strong compound lifts now
// say 4×5-8, isolations say 3×12-15, accessories sit between.
func applyTier(p *prescription, goal domain.Goal, tier exerciseTier) {
	switch tier {
	case tierHeavyCompound:
		switch goal {
		case domain.GoalStrength:
			p.repsLow, p.repsHigh, p.restSeconds = 3, 5, 180
		case domain.GoalMuscleGain:
			p.repsLow, p.repsHigh, p.restSeconds = 5, 8, 120
		case domain.GoalFatLoss:
			p.repsLow, p.repsHigh, p.restSeconds = 8, 12, 75
		case domain.GoalEndurance:
			p.repsLow, p.repsHigh, p.restSeconds = 12, 15, 60
		case domain.GoalGeneralFitness:
			p.repsLow, p.repsHigh, p.restSeconds = 6, 10, 90
		}
	case tierAccessoryCompound:
		switch goal {
		case domain.GoalStrength:
			p.repsLow, p.repsHigh, p.restSeconds = 5, 8, 120
		case domain.GoalMuscleGain:
			p.repsLow, p.repsHigh, p.restSeconds = 8, 12, 90
		case domain.GoalFatLoss:
			p.repsLow, p.repsHigh, p.restSeconds = 12, 15, 50
		case domain.GoalEndurance:
			p.repsLow, p.repsHigh, p.restSeconds = 15, 20, 40
		case domain.GoalGeneralFitness:
			p.repsLow, p.repsHigh, p.restSeconds = 8, 12, 75
		}
	case tierIsolation:
		switch goal {
		case domain.GoalStrength:
			p.repsLow, p.repsHigh, p.restSeconds = 8, 12, 75
		case domain.GoalMuscleGain:
			p.repsLow, p.repsHigh, p.restSeconds = 10, 15, 60
		case domain.GoalFatLoss:
			p.repsLow, p.repsHigh, p.restSeconds = 15, 20, 40
		case domain.GoalEndurance:
			p.repsLow, p.repsHigh, p.restSeconds = 18, 25, 30
		case domain.GoalGeneralFitness:
			p.repsLow, p.repsHigh, p.restSeconds = 12, 15, 50
		}
	}
}

func maxi(a, b int) int { if a > b { return a }; return b }

// usesExternalLoad reports whether the exercise is loaded with a
// quantifiable weight the user can adjust — barbell plates, dumbbell
// weight, machine stack pin, cable stack pin, kettlebell weight.
// Bodyweight movements (push-up, pull-up, dip, plank, scapular pull-up),
// band work, and pure-mobility exercises return false; "50% of working
// weight" is meaningless for them, so the planner skips the warmup ramp
// entirely.
func usesExternalLoad(ex domain.Exercise) bool {
	for _, eq := range ex.RequiredEquipment {
		switch eq {
		case domain.Barbell, domain.Dumbbell, domain.Cable, domain.Machine, domain.Kettlebell:
			return true
		}
	}
	return false
}

// ensureMinSetsPerMuscle bumps working-set counts so every "main muscle"
// on the day gets at least `minSets` total sets across all exercises that
// have it as primary. Modern hypertrophy research (Schoenfeld, Helms)
// recommends a minimum of 5 hard sets per muscle per session for
// progress — without this, picking 1 compound + 1 isolation can leave a
// muscle at just 3 sets, below the stimulus threshold.
//
// Allocates extra sets to the highest-volume exercise targeting each
// underweight muscle (i.e. the one carrying the most stimulus already).
func ensureMinSetsPerMuscle(exs []domain.PlanExercise, mainMuscles []domain.MuscleGroup, minSets int) {
	for _, muscle := range mainMuscles {
		// Index every exercise primarily targeting this muscle.
		idxs := make([]int, 0, 3)
		totalSets := 0
		for i, e := range exs {
			if e.Exercise.PrimaryMuscle == muscle {
				idxs = append(idxs, i)
				totalSets += e.Sets
			}
		}
		if len(idxs) == 0 || totalSets >= minSets {
			continue
		}
		need := minSets - totalSets
		// Spread extra sets round-robin starting from the last exercise
		// for this muscle (typically the isolation), since adding sets
		// there is safer than piling more onto a heavy compound.
		for need > 0 {
			for k := len(idxs) - 1; k >= 0 && need > 0; k-- {
				exs[idxs[k]].Sets++
				need--
			}
		}
	}
}

// applyWarmupContext is a session-level post-process: once a primary
// muscle has been worked by an earlier exercise in the same day, the
// second and subsequent exercises hitting that primary muscle no longer
// need their own warmup ramps. The muscle is already warm; the lighter
// working sets of an accessory ramp themselves.
//
// Example: on Push day, Bench Press (primary: chest) keeps its
// 50%/70% warmups. The Incline Bench Press and Chest Fly that follow
// also have chest as primary — applyWarmupContext nukes their warmups
// because chest is already warm from the bench work.
func applyWarmupContext(exs []domain.PlanExercise) {
	primaryHit := make(map[domain.MuscleGroup]bool)
	for i := range exs {
		pm := exs[i].Exercise.PrimaryMuscle
		if primaryHit[pm] {
			exs[i].Warmups = nil
		}
		primaryHit[pm] = true
	}
}
