package exercise

import "github.com/ataliaferro46/go-workout-api/internal/domain"

// shorthand aliases keep the seed table below readable. They are package-local
// and exist only to make the literal table scannable.
const (
	bw = domain.Bodyweight
	bb = domain.Barbell
	dm = domain.Dumbbell
	cb = domain.Cable
	mc = domain.Machine
	pb = domain.PullupBar
	bn = domain.Bench
	kb = domain.Kettlebell
	bd = domain.Bands
)

// seedExercises is the canonical seed list — the source of truth for both the
// in-memory repository (used at startup in dev/demo) and the 0003_exercises
// migration (which inserts the same rows on Postgres-mode boots). When
// expanding the library: add rows here AND write a follow-up migration. The
// migration is the source of truth at runtime; this slice is documentation
// plus the bootstrap path for non-Postgres environments.
var seedExercises = []domain.Exercise{

	// ----- Horizontal push (chest) -----
	{ID: "barbell-bench-press", Name: "Barbell Bench Press", PrimaryMuscle: domain.Chest, SecondaryMuscles: []domain.MuscleGroup{domain.Triceps, domain.Shoulders}, Pattern: domain.HorizontalPush, RequiredEquipment: []domain.Equipment{bb, bn}, Compound: true, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Shoulder}},
	{ID: "dumbbell-bench-press", Name: "Dumbbell Bench Press", PrimaryMuscle: domain.Chest, SecondaryMuscles: []domain.MuscleGroup{domain.Triceps, domain.Shoulders}, Pattern: domain.HorizontalPush, RequiredEquipment: []domain.Equipment{dm, bn}, Compound: true, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Shoulder}},
	{ID: "machine-chest-press", Name: "Machine Chest Press", PrimaryMuscle: domain.Chest, SecondaryMuscles: []domain.MuscleGroup{domain.Triceps, domain.Shoulders}, Pattern: domain.HorizontalPush, RequiredEquipment: []domain.Equipment{mc}, Compound: true, MinLevel: domain.Beginner},
	{ID: "push-up", Name: "Push-Up", PrimaryMuscle: domain.Chest, SecondaryMuscles: []domain.MuscleGroup{domain.Triceps, domain.Shoulders, domain.Core}, Pattern: domain.HorizontalPush, RequiredEquipment: []domain.Equipment{bw}, Compound: true, MinLevel: domain.Beginner},
	{ID: "incline-barbell-bench-press", Name: "Incline Barbell Bench Press", PrimaryMuscle: domain.Chest, SecondaryMuscles: []domain.MuscleGroup{domain.Shoulders, domain.Triceps}, Pattern: domain.HorizontalPush, RequiredEquipment: []domain.Equipment{bb, bn}, Compound: true, MinLevel: domain.Intermediate, Contraindications: []domain.BodyPart{domain.Shoulder}},
	{ID: "decline-barbell-bench-press", Name: "Decline Barbell Bench Press", PrimaryMuscle: domain.Chest, SecondaryMuscles: []domain.MuscleGroup{domain.Triceps}, Pattern: domain.HorizontalPush, RequiredEquipment: []domain.Equipment{bb, bn}, Compound: true, MinLevel: domain.Intermediate, Contraindications: []domain.BodyPart{domain.Shoulder}},
	{ID: "close-grip-bench-press", Name: "Close-Grip Bench Press", PrimaryMuscle: domain.Triceps, SecondaryMuscles: []domain.MuscleGroup{domain.Chest, domain.Shoulders}, Pattern: domain.HorizontalPush, RequiredEquipment: []domain.Equipment{bb, bn}, Compound: true, MinLevel: domain.Intermediate, Contraindications: []domain.BodyPart{domain.Elbow}},
	{ID: "paused-bench-press", Name: "Paused Bench Press", PrimaryMuscle: domain.Chest, SecondaryMuscles: []domain.MuscleGroup{domain.Triceps, domain.Shoulders}, Pattern: domain.HorizontalPush, RequiredEquipment: []domain.Equipment{bb, bn}, Compound: true, MinLevel: domain.Advanced, Contraindications: []domain.BodyPart{domain.Shoulder}},
	{ID: "dumbbell-incline-press", Name: "Dumbbell Incline Press", PrimaryMuscle: domain.Chest, SecondaryMuscles: []domain.MuscleGroup{domain.Shoulders, domain.Triceps}, Pattern: domain.HorizontalPush, RequiredEquipment: []domain.Equipment{dm, bn}, Compound: true, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Shoulder}},
	{ID: "dip", Name: "Dip", PrimaryMuscle: domain.Chest, SecondaryMuscles: []domain.MuscleGroup{domain.Triceps, domain.Shoulders}, Pattern: domain.HorizontalPush, RequiredEquipment: []domain.Equipment{bw}, Compound: true, MinLevel: domain.Intermediate, Contraindications: []domain.BodyPart{domain.Shoulder, domain.Elbow}},

	// ----- Vertical push (shoulders) -----
	{ID: "barbell-overhead-press", Name: "Barbell Overhead Press", PrimaryMuscle: domain.Shoulders, SecondaryMuscles: []domain.MuscleGroup{domain.Triceps}, Pattern: domain.VerticalPush, RequiredEquipment: []domain.Equipment{bb}, Compound: true, MinLevel: domain.Intermediate, Contraindications: []domain.BodyPart{domain.Shoulder, domain.LowerBack}},
	{ID: "dumbbell-shoulder-press", Name: "Dumbbell Shoulder Press", PrimaryMuscle: domain.Shoulders, SecondaryMuscles: []domain.MuscleGroup{domain.Triceps}, Pattern: domain.VerticalPush, RequiredEquipment: []domain.Equipment{dm}, Compound: true, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Shoulder}},
	{ID: "machine-shoulder-press", Name: "Machine Shoulder Press", PrimaryMuscle: domain.Shoulders, SecondaryMuscles: []domain.MuscleGroup{domain.Triceps}, Pattern: domain.VerticalPush, RequiredEquipment: []domain.Equipment{mc}, Compound: true, MinLevel: domain.Beginner},
	{ID: "pike-push-up", Name: "Pike Push-Up", PrimaryMuscle: domain.Shoulders, SecondaryMuscles: []domain.MuscleGroup{domain.Triceps}, Pattern: domain.VerticalPush, RequiredEquipment: []domain.Equipment{bw}, Compound: true, MinLevel: domain.Intermediate, Contraindications: []domain.BodyPart{domain.Shoulder}},
	{ID: "push-press", Name: "Push Press", PrimaryMuscle: domain.Shoulders, SecondaryMuscles: []domain.MuscleGroup{domain.Triceps, domain.Quads}, Pattern: domain.VerticalPush, RequiredEquipment: []domain.Equipment{bb}, Compound: true, MinLevel: domain.Intermediate, Contraindications: []domain.BodyPart{domain.Shoulder, domain.LowerBack}},
	{ID: "push-jerk", Name: "Push Jerk", PrimaryMuscle: domain.Shoulders, SecondaryMuscles: []domain.MuscleGroup{domain.Triceps, domain.Quads}, Pattern: domain.VerticalPush, RequiredEquipment: []domain.Equipment{bb}, Compound: true, MinLevel: domain.Advanced, Contraindications: []domain.BodyPart{domain.Shoulder, domain.LowerBack, domain.Knee}},
	{ID: "landmine-press", Name: "Landmine Press", PrimaryMuscle: domain.Shoulders, SecondaryMuscles: []domain.MuscleGroup{domain.Triceps, domain.Core}, Pattern: domain.VerticalPush, RequiredEquipment: []domain.Equipment{bb}, Compound: true, MinLevel: domain.Beginner},
	{ID: "z-press", Name: "Z-Press", PrimaryMuscle: domain.Shoulders, SecondaryMuscles: []domain.MuscleGroup{domain.Triceps, domain.Core}, Pattern: domain.VerticalPush, RequiredEquipment: []domain.Equipment{bb}, Compound: true, MinLevel: domain.Advanced, Contraindications: []domain.BodyPart{domain.Shoulder, domain.LowerBack}},

	// ----- Horizontal pull (back) -----
	{ID: "barbell-row", Name: "Barbell Row", PrimaryMuscle: domain.Back, SecondaryMuscles: []domain.MuscleGroup{domain.Biceps}, Pattern: domain.HorizontalPull, RequiredEquipment: []domain.Equipment{bb}, Compound: true, MinLevel: domain.Intermediate, Contraindications: []domain.BodyPart{domain.LowerBack}},
	{ID: "dumbbell-row", Name: "One-Arm Dumbbell Row", PrimaryMuscle: domain.Back, SecondaryMuscles: []domain.MuscleGroup{domain.Biceps}, Pattern: domain.HorizontalPull, RequiredEquipment: []domain.Equipment{dm}, Compound: true, MinLevel: domain.Beginner},
	{ID: "seated-cable-row", Name: "Seated Cable Row", PrimaryMuscle: domain.Back, SecondaryMuscles: []domain.MuscleGroup{domain.Biceps}, Pattern: domain.HorizontalPull, RequiredEquipment: []domain.Equipment{cb}, Compound: true, MinLevel: domain.Beginner},
	{ID: "inverted-row", Name: "Inverted Row", PrimaryMuscle: domain.Back, SecondaryMuscles: []domain.MuscleGroup{domain.Biceps, domain.Core}, Pattern: domain.HorizontalPull, RequiredEquipment: []domain.Equipment{pb}, Compound: true, MinLevel: domain.Beginner},
	{ID: "t-bar-row", Name: "T-Bar Row", PrimaryMuscle: domain.Back, SecondaryMuscles: []domain.MuscleGroup{domain.Biceps}, Pattern: domain.HorizontalPull, RequiredEquipment: []domain.Equipment{bb}, Compound: true, MinLevel: domain.Intermediate, Contraindications: []domain.BodyPart{domain.LowerBack}},
	{ID: "pendlay-row", Name: "Pendlay Row", PrimaryMuscle: domain.Back, SecondaryMuscles: []domain.MuscleGroup{domain.Biceps}, Pattern: domain.HorizontalPull, RequiredEquipment: []domain.Equipment{bb}, Compound: true, MinLevel: domain.Advanced, Contraindications: []domain.BodyPart{domain.LowerBack}},
	{ID: "chest-supported-row", Name: "Chest-Supported Row", PrimaryMuscle: domain.Back, SecondaryMuscles: []domain.MuscleGroup{domain.Biceps}, Pattern: domain.HorizontalPull, RequiredEquipment: []domain.Equipment{dm, bn}, Compound: true, MinLevel: domain.Beginner},
	{ID: "single-arm-cable-row", Name: "Single-Arm Cable Row", PrimaryMuscle: domain.Back, SecondaryMuscles: []domain.MuscleGroup{domain.Biceps}, Pattern: domain.HorizontalPull, RequiredEquipment: []domain.Equipment{cb}, Compound: true, MinLevel: domain.Beginner},

	// ----- Vertical pull (back) -----
	{ID: "pull-up", Name: "Pull-Up", PrimaryMuscle: domain.Back, SecondaryMuscles: []domain.MuscleGroup{domain.Biceps}, Pattern: domain.VerticalPull, RequiredEquipment: []domain.Equipment{pb}, Compound: true, MinLevel: domain.Intermediate},
	{ID: "lat-pulldown", Name: "Lat Pulldown", PrimaryMuscle: domain.Back, SecondaryMuscles: []domain.MuscleGroup{domain.Biceps}, Pattern: domain.VerticalPull, RequiredEquipment: []domain.Equipment{cb}, Compound: true, MinLevel: domain.Beginner},
	{ID: "assisted-pull-up", Name: "Assisted Pull-Up", PrimaryMuscle: domain.Back, SecondaryMuscles: []domain.MuscleGroup{domain.Biceps}, Pattern: domain.VerticalPull, RequiredEquipment: []domain.Equipment{mc}, Compound: true, MinLevel: domain.Beginner},
	{ID: "weighted-pull-up", Name: "Weighted Pull-Up", PrimaryMuscle: domain.Back, SecondaryMuscles: []domain.MuscleGroup{domain.Biceps}, Pattern: domain.VerticalPull, RequiredEquipment: []domain.Equipment{pb}, Compound: true, MinLevel: domain.Advanced},
	{ID: "chin-up", Name: "Chin-Up", PrimaryMuscle: domain.Back, SecondaryMuscles: []domain.MuscleGroup{domain.Biceps}, Pattern: domain.VerticalPull, RequiredEquipment: []domain.Equipment{pb}, Compound: true, MinLevel: domain.Intermediate},
	{ID: "neutral-grip-pull-up", Name: "Neutral-Grip Pull-Up", PrimaryMuscle: domain.Back, SecondaryMuscles: []domain.MuscleGroup{domain.Biceps}, Pattern: domain.VerticalPull, RequiredEquipment: []domain.Equipment{pb}, Compound: true, MinLevel: domain.Intermediate},
	{ID: "kettlebell-swing", Name: "Kettlebell Swing", PrimaryMuscle: domain.Glutes, SecondaryMuscles: []domain.MuscleGroup{domain.Hamstrings, domain.Back, domain.Core}, Pattern: domain.HingePattern, RequiredEquipment: []domain.Equipment{kb}, Compound: true, MinLevel: domain.Intermediate, Contraindications: []domain.BodyPart{domain.LowerBack}},

	// ----- Squat (quads) -----
	{ID: "barbell-back-squat", Name: "Barbell Back Squat", PrimaryMuscle: domain.Quads, SecondaryMuscles: []domain.MuscleGroup{domain.Glutes, domain.Hamstrings}, Pattern: domain.SquatPattern, RequiredEquipment: []domain.Equipment{bb}, Compound: true, MinLevel: domain.Intermediate, Contraindications: []domain.BodyPart{domain.Knee, domain.LowerBack}},
	{ID: "goblet-squat", Name: "Goblet Squat", PrimaryMuscle: domain.Quads, SecondaryMuscles: []domain.MuscleGroup{domain.Glutes}, Pattern: domain.SquatPattern, RequiredEquipment: []domain.Equipment{dm}, Compound: true, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Knee}},
	{ID: "leg-press", Name: "Leg Press", PrimaryMuscle: domain.Quads, SecondaryMuscles: []domain.MuscleGroup{domain.Glutes}, Pattern: domain.SquatPattern, RequiredEquipment: []domain.Equipment{mc}, Compound: true, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Knee}},
	{ID: "bodyweight-squat", Name: "Bodyweight Squat", PrimaryMuscle: domain.Quads, SecondaryMuscles: []domain.MuscleGroup{domain.Glutes}, Pattern: domain.SquatPattern, RequiredEquipment: []domain.Equipment{bw}, Compound: true, MinLevel: domain.Beginner},
	{ID: "front-squat", Name: "Front Squat", PrimaryMuscle: domain.Quads, SecondaryMuscles: []domain.MuscleGroup{domain.Glutes, domain.Core}, Pattern: domain.SquatPattern, RequiredEquipment: []domain.Equipment{bb}, Compound: true, MinLevel: domain.Intermediate, Contraindications: []domain.BodyPart{domain.Knee, domain.LowerBack, domain.Wrist}},
	{ID: "zercher-squat", Name: "Zercher Squat", PrimaryMuscle: domain.Quads, SecondaryMuscles: []domain.MuscleGroup{domain.Glutes, domain.Core}, Pattern: domain.SquatPattern, RequiredEquipment: []domain.Equipment{bb}, Compound: true, MinLevel: domain.Advanced, Contraindications: []domain.BodyPart{domain.Knee, domain.LowerBack, domain.Elbow}},
	{ID: "hack-squat", Name: "Hack Squat", PrimaryMuscle: domain.Quads, SecondaryMuscles: []domain.MuscleGroup{domain.Glutes}, Pattern: domain.SquatPattern, RequiredEquipment: []domain.Equipment{mc}, Compound: true, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Knee}},
	{ID: "safety-bar-squat", Name: "Safety-Bar Squat", PrimaryMuscle: domain.Quads, SecondaryMuscles: []domain.MuscleGroup{domain.Glutes, domain.Hamstrings}, Pattern: domain.SquatPattern, RequiredEquipment: []domain.Equipment{bb}, Compound: true, MinLevel: domain.Intermediate, Contraindications: []domain.BodyPart{domain.Knee, domain.LowerBack}},
	{ID: "box-squat", Name: "Box Squat", PrimaryMuscle: domain.Quads, SecondaryMuscles: []domain.MuscleGroup{domain.Glutes, domain.Hamstrings}, Pattern: domain.SquatPattern, RequiredEquipment: []domain.Equipment{bb}, Compound: true, MinLevel: domain.Intermediate, Contraindications: []domain.BodyPart{domain.LowerBack}},

	// ----- Hinge (hamstrings / glutes) -----
	{ID: "barbell-deadlift", Name: "Barbell Deadlift", PrimaryMuscle: domain.Hamstrings, SecondaryMuscles: []domain.MuscleGroup{domain.Glutes, domain.Back}, Pattern: domain.HingePattern, RequiredEquipment: []domain.Equipment{bb}, Compound: true, MinLevel: domain.Intermediate, Contraindications: []domain.BodyPart{domain.LowerBack}},
	{ID: "dumbbell-rdl", Name: "Dumbbell Romanian Deadlift", PrimaryMuscle: domain.Hamstrings, SecondaryMuscles: []domain.MuscleGroup{domain.Glutes}, Pattern: domain.HingePattern, RequiredEquipment: []domain.Equipment{dm}, Compound: true, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.LowerBack}},
	{ID: "barbell-hip-thrust", Name: "Barbell Hip Thrust", PrimaryMuscle: domain.Glutes, SecondaryMuscles: []domain.MuscleGroup{domain.Hamstrings}, Pattern: domain.HingePattern, RequiredEquipment: []domain.Equipment{bb, bn}, Compound: true, MinLevel: domain.Beginner},
	{ID: "glute-bridge", Name: "Glute Bridge", PrimaryMuscle: domain.Glutes, SecondaryMuscles: []domain.MuscleGroup{domain.Hamstrings}, Pattern: domain.HingePattern, RequiredEquipment: []domain.Equipment{bw}, Compound: true, MinLevel: domain.Beginner},
	{ID: "trap-bar-deadlift", Name: "Trap-Bar Deadlift", PrimaryMuscle: domain.Hamstrings, SecondaryMuscles: []domain.MuscleGroup{domain.Glutes, domain.Quads, domain.Back}, Pattern: domain.HingePattern, RequiredEquipment: []domain.Equipment{bb}, Compound: true, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.LowerBack}},
	{ID: "barbell-rdl", Name: "Barbell Romanian Deadlift", PrimaryMuscle: domain.Hamstrings, SecondaryMuscles: []domain.MuscleGroup{domain.Glutes, domain.Back}, Pattern: domain.HingePattern, RequiredEquipment: []domain.Equipment{bb}, Compound: true, MinLevel: domain.Intermediate, Contraindications: []domain.BodyPart{domain.LowerBack}},
	{ID: "single-leg-rdl", Name: "Single-Leg Romanian Deadlift", PrimaryMuscle: domain.Hamstrings, SecondaryMuscles: []domain.MuscleGroup{domain.Glutes}, Pattern: domain.HingePattern, RequiredEquipment: []domain.Equipment{dm}, Compound: true, MinLevel: domain.Intermediate, Contraindications: []domain.BodyPart{domain.LowerBack, domain.Ankle}},
	{ID: "clean-pull", Name: "Clean Pull", PrimaryMuscle: domain.Hamstrings, SecondaryMuscles: []domain.MuscleGroup{domain.Glutes, domain.Back, domain.Quads}, Pattern: domain.HingePattern, RequiredEquipment: []domain.Equipment{bb}, Compound: true, MinLevel: domain.Advanced, Contraindications: []domain.BodyPart{domain.LowerBack}},
	{ID: "snatch-pull", Name: "Snatch Pull", PrimaryMuscle: domain.Hamstrings, SecondaryMuscles: []domain.MuscleGroup{domain.Glutes, domain.Back, domain.Shoulders}, Pattern: domain.HingePattern, RequiredEquipment: []domain.Equipment{bb}, Compound: true, MinLevel: domain.Advanced, Contraindications: []domain.BodyPart{domain.LowerBack, domain.Shoulder}},
	{ID: "jump-shrug", Name: "Jump Shrug", PrimaryMuscle: domain.Back, SecondaryMuscles: []domain.MuscleGroup{domain.Glutes, domain.Hamstrings, domain.Calves}, Pattern: domain.HingePattern, RequiredEquipment: []domain.Equipment{bb}, Compound: true, MinLevel: domain.Advanced, Contraindications: []domain.BodyPart{domain.LowerBack, domain.Ankle}},

	// ----- Olympic lifts -----
	{ID: "power-clean", Name: "Power Clean", PrimaryMuscle: domain.Hamstrings, SecondaryMuscles: []domain.MuscleGroup{domain.Glutes, domain.Back, domain.Quads, domain.Shoulders}, Pattern: domain.HingePattern, RequiredEquipment: []domain.Equipment{bb}, Compound: true, MinLevel: domain.Advanced, Contraindications: []domain.BodyPart{domain.LowerBack, domain.Wrist, domain.Knee}},
	{ID: "hang-clean", Name: "Hang Clean", PrimaryMuscle: domain.Hamstrings, SecondaryMuscles: []domain.MuscleGroup{domain.Glutes, domain.Back, domain.Shoulders}, Pattern: domain.HingePattern, RequiredEquipment: []domain.Equipment{bb}, Compound: true, MinLevel: domain.Advanced, Contraindications: []domain.BodyPart{domain.LowerBack, domain.Wrist}},
	{ID: "power-snatch", Name: "Power Snatch", PrimaryMuscle: domain.Hamstrings, SecondaryMuscles: []domain.MuscleGroup{domain.Glutes, domain.Back, domain.Shoulders, domain.Quads}, Pattern: domain.HingePattern, RequiredEquipment: []domain.Equipment{bb}, Compound: true, MinLevel: domain.Advanced, Contraindications: []domain.BodyPart{domain.LowerBack, domain.Shoulder, domain.Wrist, domain.Knee}},
	{ID: "kettlebell-snatch", Name: "Kettlebell Snatch", PrimaryMuscle: domain.Shoulders, SecondaryMuscles: []domain.MuscleGroup{domain.Glutes, domain.Hamstrings, domain.Back}, Pattern: domain.HingePattern, RequiredEquipment: []domain.Equipment{kb}, Compound: true, MinLevel: domain.Advanced, Contraindications: []domain.BodyPart{domain.Shoulder, domain.LowerBack}},

	// ----- Lunge (quads / glutes) -----
	{ID: "dumbbell-walking-lunge", Name: "Dumbbell Walking Lunge", PrimaryMuscle: domain.Quads, SecondaryMuscles: []domain.MuscleGroup{domain.Glutes}, Pattern: domain.LungePattern, RequiredEquipment: []domain.Equipment{dm}, Compound: true, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Knee}},
	{ID: "bulgarian-split-squat", Name: "Bulgarian Split Squat", PrimaryMuscle: domain.Quads, SecondaryMuscles: []domain.MuscleGroup{domain.Glutes}, Pattern: domain.LungePattern, RequiredEquipment: []domain.Equipment{dm, bn}, Compound: true, MinLevel: domain.Intermediate, Contraindications: []domain.BodyPart{domain.Knee}},
	{ID: "reverse-lunge", Name: "Bodyweight Reverse Lunge", PrimaryMuscle: domain.Quads, SecondaryMuscles: []domain.MuscleGroup{domain.Glutes}, Pattern: domain.LungePattern, RequiredEquipment: []domain.Equipment{bw}, Compound: true, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Knee}},
	{ID: "step-up", Name: "Dumbbell Step-Up", PrimaryMuscle: domain.Quads, SecondaryMuscles: []domain.MuscleGroup{domain.Glutes}, Pattern: domain.LungePattern, RequiredEquipment: []domain.Equipment{dm, bn}, Compound: true, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Knee}},
	{ID: "cossack-squat", Name: "Cossack Squat", PrimaryMuscle: domain.Quads, SecondaryMuscles: []domain.MuscleGroup{domain.Glutes, domain.Hamstrings}, Pattern: domain.LungePattern, RequiredEquipment: []domain.Equipment{bw}, Compound: true, MinLevel: domain.Intermediate, Contraindications: []domain.BodyPart{domain.Knee, domain.Hip, domain.Ankle}},

	// ----- Isolation: arms -----
	{ID: "dumbbell-curl", Name: "Dumbbell Biceps Curl", PrimaryMuscle: domain.Biceps, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{dm}, Compound: false, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Elbow}},
	{ID: "cable-curl", Name: "Cable Biceps Curl", PrimaryMuscle: domain.Biceps, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{cb}, Compound: false, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Elbow}},
	{ID: "tricep-pushdown", Name: "Cable Triceps Pushdown", PrimaryMuscle: domain.Triceps, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{cb}, Compound: false, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Elbow}},
	{ID: "bench-dip", Name: "Bench Triceps Dip", PrimaryMuscle: domain.Triceps, SecondaryMuscles: []domain.MuscleGroup{domain.Chest}, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{bw, bn}, Compound: false, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Shoulder, domain.Elbow}},
	{ID: "hammer-curl", Name: "Hammer Curl", PrimaryMuscle: domain.Biceps, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{dm}, Compound: false, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Elbow}},
	{ID: "concentration-curl", Name: "Concentration Curl", PrimaryMuscle: domain.Biceps, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{dm, bn}, Compound: false, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Elbow}},
	{ID: "lying-triceps-extension", Name: "Lying Triceps Extension", PrimaryMuscle: domain.Triceps, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{bb, bn}, Compound: false, MinLevel: domain.Intermediate, Contraindications: []domain.BodyPart{domain.Elbow, domain.Shoulder}},

	// ----- Isolation: shoulders / chest / legs -----
	{ID: "lateral-raise", Name: "Dumbbell Lateral Raise", PrimaryMuscle: domain.Shoulders, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{dm}, Compound: false, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Shoulder}},
	{ID: "cable-lateral-raise", Name: "Cable Lateral Raise", PrimaryMuscle: domain.Shoulders, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{cb}, Compound: false, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Shoulder}},
	{ID: "face-pull", Name: "Cable Face Pull", PrimaryMuscle: domain.Shoulders, SecondaryMuscles: []domain.MuscleGroup{domain.Back}, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{cb}, Compound: false, MinLevel: domain.Beginner},
	{ID: "cable-reverse-fly", Name: "Cable Reverse Fly", PrimaryMuscle: domain.Shoulders, SecondaryMuscles: []domain.MuscleGroup{domain.Back}, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{cb}, Compound: false, MinLevel: domain.Beginner},
	{ID: "dumbbell-fly", Name: "Dumbbell Chest Fly", PrimaryMuscle: domain.Chest, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{dm, bn}, Compound: false, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Shoulder}},
	{ID: "leg-extension", Name: "Machine Leg Extension", PrimaryMuscle: domain.Quads, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{mc}, Compound: false, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Knee}},
	{ID: "leg-curl", Name: "Machine Leg Curl", PrimaryMuscle: domain.Hamstrings, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{mc}, Compound: false, MinLevel: domain.Beginner},
	{ID: "dumbbell-calf-raise", Name: "Dumbbell Calf Raise", PrimaryMuscle: domain.Calves, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{dm}, Compound: false, MinLevel: domain.Beginner},
	{ID: "standing-calf-raise", Name: "Standing Calf Raise", PrimaryMuscle: domain.Calves, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{bw}, Compound: false, MinLevel: domain.Beginner},
	{ID: "leg-press-calf-raise", Name: "Leg Press Calf Raise", PrimaryMuscle: domain.Calves, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{mc}, Compound: false, MinLevel: domain.Beginner},

	// ----- Core -----
	{ID: "plank", Name: "Plank", PrimaryMuscle: domain.Core, Pattern: domain.CorePattern, RequiredEquipment: []domain.Equipment{bw}, Compound: false, MinLevel: domain.Beginner},
	{ID: "hanging-leg-raise", Name: "Hanging Leg Raise", PrimaryMuscle: domain.Core, Pattern: domain.CorePattern, RequiredEquipment: []domain.Equipment{pb}, Compound: false, MinLevel: domain.Intermediate},
	{ID: "cable-crunch", Name: "Cable Crunch", PrimaryMuscle: domain.Core, Pattern: domain.CorePattern, RequiredEquipment: []domain.Equipment{cb}, Compound: false, MinLevel: domain.Beginner},
	{ID: "dead-bug", Name: "Dead Bug", PrimaryMuscle: domain.Core, Pattern: domain.CorePattern, RequiredEquipment: []domain.Equipment{bw}, Compound: false, MinLevel: domain.Beginner},
	{ID: "weighted-plank", Name: "Weighted Plank", PrimaryMuscle: domain.Core, Pattern: domain.CorePattern, RequiredEquipment: []domain.Equipment{bw}, Compound: false, MinLevel: domain.Intermediate},
	{ID: "hanging-windshield-wiper", Name: "Hanging Windshield Wiper", PrimaryMuscle: domain.Core, SecondaryMuscles: []domain.MuscleGroup{domain.Back}, Pattern: domain.CorePattern, RequiredEquipment: []domain.Equipment{pb}, Compound: false, MinLevel: domain.Advanced},

	// ----- Plyometric + conditioning -----
	{ID: "box-jump", Name: "Box Jump", PrimaryMuscle: domain.Quads, SecondaryMuscles: []domain.MuscleGroup{domain.Glutes, domain.Calves}, Pattern: domain.SquatPattern, RequiredEquipment: []domain.Equipment{bw}, Compound: true, MinLevel: domain.Intermediate, Contraindications: []domain.BodyPart{domain.Knee, domain.Ankle}},
	{ID: "broad-jump", Name: "Broad Jump", PrimaryMuscle: domain.Quads, SecondaryMuscles: []domain.MuscleGroup{domain.Glutes, domain.Hamstrings}, Pattern: domain.SquatPattern, RequiredEquipment: []domain.Equipment{bw}, Compound: true, MinLevel: domain.Intermediate, Contraindications: []domain.BodyPart{domain.Knee, domain.Ankle}},
	{ID: "depth-jump", Name: "Depth Jump", PrimaryMuscle: domain.Quads, SecondaryMuscles: []domain.MuscleGroup{domain.Glutes, domain.Calves}, Pattern: domain.SquatPattern, RequiredEquipment: []domain.Equipment{bw}, Compound: true, MinLevel: domain.Advanced, Contraindications: []domain.BodyPart{domain.Knee, domain.Ankle}},
	{ID: "farmers-walk", Name: "Farmer's Walk", PrimaryMuscle: domain.Core, SecondaryMuscles: []domain.MuscleGroup{domain.Back, domain.Calves}, Pattern: domain.CorePattern, RequiredEquipment: []domain.Equipment{dm}, Compound: true, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.LowerBack}},
	{ID: "jump-rope", Name: "Jump Rope", PrimaryMuscle: domain.Calves, SecondaryMuscles: []domain.MuscleGroup{domain.Core}, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{bw}, Compound: false, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Ankle}},

	// ----- Mobility + accessory -----
	{ID: "cuban-rotation", Name: "Cuban Rotation", PrimaryMuscle: domain.Shoulders, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{dm}, Compound: false, MinLevel: domain.Beginner},
	{ID: "band-pull-apart", Name: "Band Pull-Apart", PrimaryMuscle: domain.Back, SecondaryMuscles: []domain.MuscleGroup{domain.Shoulders}, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{bd}, Compound: false, MinLevel: domain.Beginner},
	{ID: "scapular-pull-up", Name: "Scapular Pull-Up", PrimaryMuscle: domain.Back, SecondaryMuscles: []domain.MuscleGroup{domain.Shoulders}, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{pb}, Compound: false, MinLevel: domain.Beginner},
	{ID: "dead-hang", Name: "Dead Hang", PrimaryMuscle: domain.Back, SecondaryMuscles: []domain.MuscleGroup{domain.Shoulders, domain.Core}, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{pb}, Compound: false, MinLevel: domain.Beginner},
	{ID: "sissy-squat", Name: "Sissy Squat", PrimaryMuscle: domain.Quads, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{bw}, Compound: false, MinLevel: domain.Intermediate, Contraindications: []domain.BodyPart{domain.Knee}, Region: "vasti"},

	// ----- Head/region-specific accessories (added with ADR-068 muscle
	// regions; see internal/domain/exercise.go and migration 0008). These
	// fill consensus gaps in the original library — overhead extension for
	// the long head of the triceps, preacher curl for the short head of
	// the biceps, rear delt fly for the most underdeveloped delt head,
	// seated calf raise for the soleus, pullover for the lower lats, and
	// the barbell hip thrust for direct glute work.
	{ID: "overhead-tricep-extension", Name: "Cable Overhead Triceps Extension", PrimaryMuscle: domain.Triceps, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{cb}, Compound: false, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Elbow, domain.Shoulder}, Region: "long_head"},
	{ID: "tricep-kickback", Name: "Dumbbell Triceps Kickback", PrimaryMuscle: domain.Triceps, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{dm}, Compound: false, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Elbow}, Region: "lateral_head"},
	{ID: "preacher-curl", Name: "Preacher Curl", PrimaryMuscle: domain.Biceps, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{dm, bn}, Compound: false, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Elbow}, Region: "short_head"},
	{ID: "incline-dumbbell-curl", Name: "Incline Dumbbell Curl", PrimaryMuscle: domain.Biceps, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{dm, bn}, Compound: false, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Elbow}, Region: "long_head"},
	{ID: "rear-delt-fly", Name: "Dumbbell Rear Delt Fly", PrimaryMuscle: domain.Shoulders, SecondaryMuscles: []domain.MuscleGroup{domain.Back}, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{dm}, Compound: false, MinLevel: domain.Beginner, Region: "rear_delt"},
	{ID: "seated-calf-raise", Name: "Seated Calf Raise", PrimaryMuscle: domain.Calves, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{mc}, Compound: false, MinLevel: domain.Beginner, Region: "soleus"},
	{ID: "romanian-deadlift-iso", Name: "Single-Leg Romanian Deadlift", PrimaryMuscle: domain.Hamstrings, SecondaryMuscles: []domain.MuscleGroup{domain.Glutes}, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{dm}, Compound: false, MinLevel: domain.Intermediate, Contraindications: []domain.BodyPart{domain.LowerBack}, Region: "hip_extension"},
	{ID: "cable-pullover", Name: "Cable Pullover", PrimaryMuscle: domain.Back, SecondaryMuscles: []domain.MuscleGroup{domain.Chest}, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{cb}, Compound: false, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Shoulder}, Region: "lats_lower"},
	{ID: "straight-arm-pulldown", Name: "Straight-Arm Pulldown", PrimaryMuscle: domain.Back, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{cb}, Compound: false, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Shoulder}, Region: "lats_lower"},
	{ID: "incline-cable-fly", Name: "Incline Cable Fly", PrimaryMuscle: domain.Chest, SecondaryMuscles: []domain.MuscleGroup{domain.Shoulders}, Pattern: domain.Isolation, RequiredEquipment: []domain.Equipment{cb}, Compound: false, MinLevel: domain.Beginner, Contraindications: []domain.BodyPart{domain.Shoulder}, Region: "upper_chest"},
	{ID: "hip-thrust", Name: "Barbell Hip Thrust", PrimaryMuscle: domain.Glutes, SecondaryMuscles: []domain.MuscleGroup{domain.Hamstrings}, Pattern: domain.HingePattern, RequiredEquipment: []domain.Equipment{bb, bn}, Compound: true, MinLevel: domain.Beginner, Region: "glute_max"},
}
