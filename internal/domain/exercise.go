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

	// Region is an anatomical sub-target within the PrimaryMuscle, used by
	// the plan engine to rotate which "head" or "fiber group" gets worked
	// across the week. Empty means "no regional preference / hits the whole
	// muscle." See docs/specs/muscle_regions.md for the consensus mapping;
	// values are stable strings so they can be checked from the planner
	// without an enum import cycle.
	//
	//   Triceps    → "long_head"   (arm-overhead bias: skullcrusher, overhead ext)
	//                "lateral_head" (arm-at-side: pushdown, kickback)
	//                "all_heads"   (close-grip pressing, dips)
	//   Chest      → "upper_chest" (incline ≥15°)
	//                "mid_chest"   (flat fly, flat press)
	//                "lower_chest" (decline, dips)
	//   Biceps     → "long_head"   (incline curl, behind-body)
	//                "short_head"  (preacher, concentration)
	//                "brachialis"  (hammer, reverse curl)
	//   Shoulders  → "front_delt"  (pressing — usually compounds)
	//                "lateral_delt"(lateral raise variants)
	//                "rear_delt"   (face pull, rear delt fly)
	//   Back       → "lats_width"  (vertical pulls)
	//                "mid_back"    (horizontal pulls / rhomboids)
	//                "lats_lower"  (pullover / straight-arm pulldown)
	//   Hamstrings → "knee_flexion"(leg curls)
	//                "hip_extension"(RDL, good morning)
	//   Calves     → "gastrocnemius"(standing — knee straight)
	//                "soleus"      (seated — knee bent)
	Region string `json:"region,omitempty"`
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
