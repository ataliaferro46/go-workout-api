package domain

// Exercise is a movement in the library — a template the engine can prescribe.
// (Distinct from LoggedExercise in workout.go, which records what a user
// actually did in a session.)
type Exercise struct {
	ID                string          `json:"id"`
	Name              string          `json:"name"`
	PrimaryMuscle     MuscleGroup     `json:"primary_muscle"`
	SecondaryMuscles  []MuscleGroup   `json:"secondary_muscles,omitempty"`
	Pattern           MovementPattern `json:"pattern"`
	RequiredEquipment []Equipment     `json:"required_equipment"`
	Compound          bool            `json:"compound"`
	MinLevel          ExperienceLevel `json:"min_level"`
	Contraindications []BodyPart      `json:"contraindications,omitempty"`
}

// RequiresOnly reports whether every piece of equipment this exercise needs is
// present in the available set. Bodyweight is always considered available.
func (e Exercise) RequiresOnly(available map[Equipment]bool) bool {
	for _, req := range e.RequiredEquipment {
		if req == Bodyweight {
			continue
		}
		if !available[req] {
			return false
		}
	}
	return true
}

// ConflictsWith reports whether this exercise is contraindicated for any of the
// user's active injuries.
func (e Exercise) ConflictsWith(injuries map[BodyPart]bool) bool {
	for _, c := range e.Contraindications {
		if injuries[c] {
			return true
		}
	}
	return false
}

// TargetsAny reports whether the exercise's primary or secondary muscles
// intersect the given set.
func (e Exercise) TargetsAny(muscles map[MuscleGroup]bool) bool {
	if muscles[e.PrimaryMuscle] {
		return true
	}
	for _, m := range e.SecondaryMuscles {
		if muscles[m] {
			return true
		}
	}
	return false
}
