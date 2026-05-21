// Package exercise provides the seed exercise library. In a production system
// this data would live in Postgres and be loaded through a repository; keeping
// it as a typed slice here makes the engine runnable and testable with no
// external dependencies.
package exercise

import "github.com/ataliaferro46/go-workout-api/internal/domain"

// Library returns a fresh copy of the seed exercise library. Each call returns
// a new slice so callers cannot mutate shared state.
func Library() []domain.Exercise {
	src := library
	out := make([]domain.Exercise, len(src))
	copy(out, src)
	return out
}

// shorthand aliases to keep the table below readable.
const (
	bw = domain.Bodyweight
	bb = domain.Barbell
	db = domain.Dumbbell
	cb = domain.Cable
	mc = domain.Machine
	pb = domain.PullupBar
	bn = domain.Bench
)

var library = []domain.Exercise{
	// ---- Horizontal push (chest) ----
	{ID: "barbell-bench-press", Name: "Barbell Bench Press", PrimaryMuscle: domain.Chest, SecondaryMuscles: []domain.MuscleGroup{domain.Triceps, domain.Shoulders}, Pattern: domain.HorizontalPush, RequiredEquipment: []domain.Equipment{bb, bn}, Compound: true, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Shoulder}},
	{ID: "dumbbell-bench-press", Name: "Dumbbell Bench Press", PrimaryMuscle: domain.Chest, SecondaryMuscles: []domain.MuscleGroup{domain.Triceps, domain.Shoulders}, Pattern: domain.HorizontalPush, RequiredEquipment: []domain.Equipment{db, bn}, Compound: true, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Shoulder}},
	{ID: "machine-chest-press", Name: "Machine Chest Press", PrimaryMuscle: domain.Chest, SecondaryMuscles: []domain.MuscleGroup{domain.Triceps, domain.Shoulders}, Pattern: domain.HorizontalPush, RequiredEquipment: []domain.Equipment{mc}, Compound: true, MinLevel: domain.Beginner},
	{ID: "push-up", Name: "Push-Up", PrimaryMuscle: domain.Chest, SecondaryMuscles: []domain.MuscleGroup{domain.Triceps, domain.Shoulders, domain.Core}, Pattern: domain.HorizontalPush, RequiredEquipment: []domain.Equipment{bw}, Compound: true, MinLevel: domain.Beginner},

	// ---- Vertical push (shoulders) ----
	{ID: "barbell-overhead-press", Name: "Barbell Overhead Press", PrimaryMuscle: domain.Shoulders, SecondaryMuscles: []domain.MuscleGroup{domain.Triceps}, Pattern: domain.VerticalPush, RequiredEquipment: []domain.Equipment{bb}, Compound: true, MinLevel: domain.Intermediate, Contraindications: []domain.BodyPart{domain.Shoulder, domain.LowerBack}},
	{ID: "dumbbell-shoulder-press", Name: "Dumbbell Shoulder Press", PrimaryMuscle: domain.Shoulders, SecondaryMuscles: []domain.MuscleGroup{domain.Triceps}, Pattern: domain.VerticalPush, RequiredEquipment: []domain.Equipment{db}, Compound: true, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Shoulder}},
	{ID: "machine-shoulder-press", Name: "Machine Shoulder Press", PrimaryMuscle: domain.Shoulders, SecondaryMuscles: []domain.MuscleGroup{domain.Triceps}, Pattern: domain.VerticalPush, RequiredEquipment: []domain.Equipment{mc}, Compound: true, MinLevel: domain.Beginner},
	{ID: "pike-push-up", Name: "Pike Push-Up", PrimaryMuscle: domain.Shoulders, SecondaryMuscles: []domain.MuscleGroup{domain.Triceps}, Pattern: domain.VerticalPush, RequiredEquipment: []domain.Equipment{bw}, Compound: true, MinLevel: domain.Intermediate, Contraindications: []domain.BodyPart{domain.Shoulder}},

	// ---- Horizontal pull (back) ----
	{ID: "barbell-row", Name: "Barbell Row", PrimaryMuscle: domain.Back, SecondaryMuscles: []domain.MuscleGroup{domain.Biceps}, Pattern: domain.HorizontalPull, RequiredEquipment: []domain.Equipment{bb}, Compound: true, MinLevel: domain.Intermediate, Contraindications: []domain.BodyPart{domain.LowerBack}},
	{ID: "dumbbell-row", Name: "One-Arm Dumbbell Row", PrimaryMuscle: domain.Back, SecondaryMuscles: []domain.MuscleGroup{domain.Biceps}, Pattern: domain.HorizontalPull, RequiredEquipment: []domain.Equipment{db}, Compound: true, MinLevel: domain.Beginner},
	{ID: "seated-cable-row", Name: "Seated Cable Row", PrimaryMuscle: domain.Back, SecondaryMuscles: []domain.MuscleGroup{domain.Biceps}, Pattern: domain.HorizontalPull, RequiredEquipment: []domain.Equipment{cb}, Compound: true, MinLevel: domain.Beginner},
	{ID: "inverted-row", Name: "Inverted Row", PrimaryMuscle: domain.Back, SecondaryMuscles: []domain.MuscleGroup{domain.Biceps, domain.Core}, Pattern: domain.HorizontalPull, RequiredEquipment: []domain.Equipment{pb}, Compound: true, MinLevel: domain.Beginner},

	// ---- Vertical pull (back) ----
	{ID: "pull-up", Name: "Pull-Up", PrimaryMuscle: domain.Back, SecondaryMuscles: []domain.MuscleGroup{domain.Biceps}, Pattern: domain.VerticalPull, RequiredEquipment: []domain.Equipment{pb}, Compound: true, MinLevel: domain.Intermediate},
	{ID: "lat-pulldown", Name: "Lat Pulldown", PrimaryMuscle: domain.Back, SecondaryMuscles: []domain.MuscleGroup{domain.Biceps}, Pattern: domain.VerticalPull, RequiredEquipment: []domain.Equipment{cb}, Compound: true, MinLevel: domain.Beginner},
	{ID: "assisted-pull-up", Name: "Assisted Pull-Up", PrimaryMuscle: domain.Back, SecondaryMuscles: []domain.MuscleGroup{domain.Biceps}, Pattern: domain.VerticalPull, RequiredEquipment: []domain.Equipment{mc}, Compound: true, MinLevel: domain.Beginner},

	// ---- Squat (quads) ----
	{ID: "barbell-back-squat", Name: "Barbell Back Squat", PrimaryMuscle: domain.Quads, SecondaryMuscles: []domain.MuscleGroup{domain.Glutes, domain.Hamstrings}, Pattern: domain.SquatPattern, RequiredEquipment: []domain.Equipment{bb}, Compound: true, MinLevel: domain.Intermediate, Contraindications: []domain.BodyPart{domain.Knee, domain.LowerBack}},
	{ID: "goblet-squat", Name: "Goblet Squat", PrimaryMuscle: domain.Quads, SecondaryMuscles: []domain.MuscleGroup{domain.Glutes}, Pattern: domain.SquatPattern, RequiredEquipment: []domain.Equipment{db}, Compound: true, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Knee}},
	{ID: "leg-press", Name: "Leg Press", PrimaryMuscle: domain.Quads, SecondaryMuscles: []domain.MuscleGroup{domain.Glutes}, Pattern: domain.SquatPattern, RequiredEquipment: []domain.Equipment{mc}, Compound: true, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Knee}},
	{ID: "bodyweight-squat", Name: "Bodyweight Squat", PrimaryMuscle: domain.Quads, SecondaryMuscles: []domain.MuscleGroup{domain.Glutes}, Pattern: domain.SquatPattern, RequiredEquipment: []domain.Equipment{bw}, Compound: true, MinLevel: domain.Beginner},

	// ---- Hinge (hamstrings / glutes) ----
	{ID: "barbell-deadlift", Name: "Barbell Deadlift", PrimaryMuscle: domain.Hamstrings, SecondaryMuscles: []domain.MuscleGroup{domain.Glutes, domain.Back}, Pattern: domain.HingePattern, RequiredEquipment: []domain.Equipment{bb}, Compound: true, MinLevel: domain.Intermediate, Contraindications: []domain.BodyPart{domain.LowerBack}},
	{ID: "dumbbell-rdl", Name: "Dumbbell Romanian Deadlift", PrimaryMuscle: domain.Hamstrings, SecondaryMuscles: []domain.MuscleGroup{domain.Glutes}, Pattern: domain.HingePattern, RequiredEquipment: []domain.Equipment{db}, Compound: true, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.LowerBack}},
	{ID: "barbell-hip-thrust", Name: "Barbell Hip Thrust", PrimaryMuscle: domain.Glutes, SecondaryMuscles: []domain.MuscleGroup{domain.Hamstrings}, Pattern: domain.HingePattern, RequiredEquipment: []domain.Equipment{bb, bn}, Compound: true, MinLevel: domain.Beginner},
	{ID: "glute-bridge", Name: "Glute Bridge", PrimaryMuscle: domain.Glutes, SecondaryMuscles: []domain.MuscleGroup{domain.Hamstrings}, Pattern: domain.HingePattern, RequiredEquipment: []domain.Equipment{bw}, Compound: true, MinLevel: domain.Beginner},

	// ---- Lunge (quads / glutes) ----
	{ID: "dumbbell-walking-lunge", Name: "Dumbbell Walking Lunge", PrimaryMuscle: domain.Quads, SecondaryMuscles: []domain.MuscleGroup{domain.Glutes}, Pattern: domain.LungePattern, RequiredEquipment: []domain.Equipment{db}, Compound: true, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Knee}},
	{ID: "bulgarian-split-squat", Name: "Bulgarian Split Squat", PrimaryMuscle: domain.Quads, SecondaryMuscles: []domain.MuscleGroup{domain.Glutes}, Pattern: domain.LungePattern, RequiredEquipment: []domain.Equipment{db, bn}, Compound: true, MinLevel: domain.Intermediate, Contraindications: []domain.BodyPart{domain.Knee}},
	{ID: "reverse-lunge", Name: "Bodyweight Reverse Lunge", PrimaryMuscle: domain.Quads, SecondaryMuscles: []domain.MuscleGroup{domain.Glutes}, Pattern: domain.LungePattern, RequiredEquipment: []domain.Equipment{bw}, Compound: true, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Knee}},

	// ---- Isolation: arms ----
	{ID: "dumbbell-curl", Name: "Dumbbell Biceps Curl", PrimaryMuscle: domain.Biceps, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{db}, Compound: false, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Elbow}},
	{ID: "cable-curl", Name: "Cable Biceps Curl", PrimaryMuscle: domain.Biceps, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{cb}, Compound: false, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Elbow}},
	{ID: "tricep-pushdown", Name: "Cable Triceps Pushdown", PrimaryMuscle: domain.Triceps, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{cb}, Compound: false, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Elbow}},
	{ID: "bench-dip", Name: "Bench Triceps Dip", PrimaryMuscle: domain.Triceps, SecondaryMuscles: []domain.MuscleGroup{domain.Chest}, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{bw, bn}, Compound: false, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Shoulder, domain.Elbow}},

	// ---- Isolation: shoulders / chest / legs ----
	{ID: "lateral-raise", Name: "Dumbbell Lateral Raise", PrimaryMuscle: domain.Shoulders, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{db}, Compound: false, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Shoulder}},
	{ID: "face-pull", Name: "Cable Face Pull", PrimaryMuscle: domain.Shoulders, SecondaryMuscles: []domain.MuscleGroup{domain.Back}, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{cb}, Compound: false, MinLevel: domain.Beginner},
	{ID: "dumbbell-fly", Name: "Dumbbell Chest Fly", PrimaryMuscle: domain.Chest, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{db, bn}, Compound: false, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Shoulder}},
	{ID: "leg-extension", Name: "Machine Leg Extension", PrimaryMuscle: domain.Quads, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{mc}, Compound: false, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Knee}},
	{ID: "leg-curl", Name: "Machine Leg Curl", PrimaryMuscle: domain.Hamstrings, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{mc}, Compound: false, MinLevel: domain.Beginner},
	{ID: "dumbbell-calf-raise", Name: "Dumbbell Calf Raise", PrimaryMuscle: domain.Calves, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{db}, Compound: false, MinLevel: domain.Beginner},
	{ID: "standing-calf-raise", Name: "Standing Calf Raise", PrimaryMuscle: domain.Calves, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{bw}, Compound: false, MinLevel: domain.Beginner},

	// ---- Core ----
	{ID: "plank", Name: "Plank", PrimaryMuscle: domain.Core, Pattern: domain.CorePattern, RequiredEquipment: []domain.Equipment{bw}, Compound: false, MinLevel: domain.Beginner},
	{ID: "hanging-leg-raise", Name: "Hanging Leg Raise", PrimaryMuscle: domain.Core, Pattern: domain.CorePattern, RequiredEquipment: []domain.Equipment{pb}, Compound: false, MinLevel: domain.Intermediate},
	{ID: "cable-crunch", Name: "Cable Crunch", PrimaryMuscle: domain.Core, Pattern: domain.CorePattern, RequiredEquipment: []domain.Equipment{cb}, Compound: false, MinLevel: domain.Beginner},
	{ID: "dead-bug", Name: "Dead Bug", PrimaryMuscle: domain.Core, Pattern: domain.CorePattern, RequiredEquipment: []domain.Equipment{bw}, Compound: false, MinLevel: domain.Beginner},
}
