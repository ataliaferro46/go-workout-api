package plan

import "github.com/ataliaferro46/go-workout-api/internal/domain"

// dayTemplate describes one training day's intent: a display name, the ordered
// movement patterns to prioritize (compounds are chosen for these first), and
// the muscle groups the day should cover.
type dayTemplate struct {
	Name     string
	Patterns []domain.MovementPattern
	Muscles  []domain.MuscleGroup
}

// splitPlan pairs a human-readable split name with its per-day templates.
type splitPlan struct {
	Name string
	Days []dayTemplate
}

// Reusable day templates.
var (
	pushDay = dayTemplate{
		Name:     "Push",
		Patterns: []domain.MovementPattern{domain.HorizontalPush, domain.VerticalPush, domain.Isolation, domain.Isolation},
		Muscles:  []domain.MuscleGroup{domain.Chest, domain.Shoulders, domain.Triceps},
	}
	pullDay = dayTemplate{
		Name:     "Pull",
		Patterns: []domain.MovementPattern{domain.VerticalPull, domain.HorizontalPull, domain.Isolation, domain.Isolation},
		Muscles:  []domain.MuscleGroup{domain.Back, domain.Biceps},
	}
	legsDay = dayTemplate{
		Name:     "Legs",
		Patterns: []domain.MovementPattern{domain.SquatPattern, domain.HingePattern, domain.LungePattern, domain.Isolation, domain.CorePattern},
		Muscles:  []domain.MuscleGroup{domain.Quads, domain.Hamstrings, domain.Glutes, domain.Calves},
	}
	upperDay = dayTemplate{
		Name:     "Upper",
		Patterns: []domain.MovementPattern{domain.HorizontalPush, domain.VerticalPull, domain.VerticalPush, domain.HorizontalPull, domain.Isolation},
		Muscles:  []domain.MuscleGroup{domain.Chest, domain.Back, domain.Shoulders, domain.Biceps, domain.Triceps},
	}
	lowerDay = dayTemplate{
		Name:     "Lower",
		Patterns: []domain.MovementPattern{domain.SquatPattern, domain.HingePattern, domain.LungePattern, domain.Isolation, domain.CorePattern},
		Muscles:  []domain.MuscleGroup{domain.Quads, domain.Hamstrings, domain.Glutes, domain.Calves},
	}
	fullBody = dayTemplate{
		Name:     "Full Body",
		Patterns: []domain.MovementPattern{domain.SquatPattern, domain.HorizontalPush, domain.VerticalPull, domain.HingePattern, domain.CorePattern},
		Muscles:  []domain.MuscleGroup{domain.Quads, domain.Chest, domain.Back, domain.Hamstrings, domain.Core},
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
