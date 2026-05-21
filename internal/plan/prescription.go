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
// and experience level.
func prescribe(req domain.GenerateRequest, ex domain.Exercise) prescription {
	p, ok := goalScheme[req.Goal]
	if !ok {
		p = goalScheme[domain.GoalGeneralFitness]
	}

	// Compounds carry more of the stimulus, so they get an extra working set;
	// isolation movements get one fewer.
	if ex.Compound {
		p.sets++
	} else if p.sets > 2 {
		p.sets--
	}

	// Very low rep ranges don't suit isolation lifts; nudge them toward a
	// hypertrophy range even on a strength program.
	if req.Goal == domain.GoalStrength && !ex.Compound {
		p.repsLow, p.repsHigh = 8, 12
		p.restSeconds = 90
	}

	// Scale volume by experience: beginners recover less and benefit from less
	// volume; advanced lifters tolerate more.
	switch req.Experience {
	case domain.Beginner:
		if p.sets > 3 {
			p.sets = 3
		}
	case domain.Advanced:
		if ex.Compound {
			p.sets++
		}
	}

	if p.sets < 2 {
		p.sets = 2
	}
	return p
}
