// Package domain holds the core types and invariants for the fitness backend:
// the exercise library, plan generation, and logged workout sessions. It
// imports no other internal package, so it sits at the bottom of the
// dependency graph.
package domain

// Goal is the user's primary training objective. It drives how sets, reps, and
// rest are prescribed (see the prescription logic in the plan package).
type Goal string

const (
	GoalFatLoss        Goal = "fat_loss"
	GoalMuscleGain     Goal = "muscle_gain"
	GoalStrength       Goal = "strength"
	GoalEndurance      Goal = "endurance"
	GoalGeneralFitness Goal = "general_fitness"
)

// Valid reports whether g is a recognized goal.
func (g Goal) Valid() bool {
	switch g {
	case GoalFatLoss, GoalMuscleGain, GoalStrength, GoalEndurance, GoalGeneralFitness:
		return true
	default:
		return false
	}
}

// ExperienceLevel gates which exercises a user can be prescribed and scales
// training volume.
type ExperienceLevel string

const (
	Beginner     ExperienceLevel = "beginner"
	Intermediate ExperienceLevel = "intermediate"
	Advanced     ExperienceLevel = "advanced"
)

// rank converts an experience level to an ordinal for comparison.
func (e ExperienceLevel) rank() int {
	switch e {
	case Beginner:
		return 0
	case Intermediate:
		return 1
	case Advanced:
		return 2
	default:
		return -1
	}
}

// Valid reports whether e is a recognized experience level.
func (e ExperienceLevel) Valid() bool { return e.rank() >= 0 }

// CanPerform reports whether a user at level e may perform an exercise whose
// minimum required level is min.
func (e ExperienceLevel) CanPerform(min ExperienceLevel) bool {
	return e.rank() >= min.rank()
}

// MuscleGroup is a target muscle used for balancing coverage across a plan.
type MuscleGroup string

const (
	Chest      MuscleGroup = "chest"
	Back       MuscleGroup = "back"
	Shoulders  MuscleGroup = "shoulders"
	Biceps     MuscleGroup = "biceps"
	Triceps    MuscleGroup = "triceps"
	Quads      MuscleGroup = "quads"
	Hamstrings MuscleGroup = "hamstrings"
	Glutes     MuscleGroup = "glutes"
	Calves     MuscleGroup = "calves"
	Core       MuscleGroup = "core"
)

// MovementPattern classifies an exercise by the movement it trains. Splits are
// expressed in terms of patterns so the engine can balance pushing, pulling,
// squatting, and hinging.
type MovementPattern string

const (
	HorizontalPush MovementPattern = "horizontal_push"
	VerticalPush   MovementPattern = "vertical_push"
	HorizontalPull MovementPattern = "horizontal_pull"
	VerticalPull   MovementPattern = "vertical_pull"
	SquatPattern   MovementPattern = "squat"
	HingePattern   MovementPattern = "hinge"
	LungePattern   MovementPattern = "lunge"
	CorePattern    MovementPattern = "core"
	Isolation      MovementPattern = "isolation"
)

// Equipment is a piece of gear an exercise requires. Bodyweight is treated as
// always available by the engine.
type Equipment string

const (
	Barbell    Equipment = "barbell"
	Dumbbell   Equipment = "dumbbell"
	Cable      Equipment = "cable"
	Machine    Equipment = "machine"
	Kettlebell Equipment = "kettlebell"
	Bands      Equipment = "bands"
	PullupBar  Equipment = "pullup_bar"
	Bench      Equipment = "bench"
	Bodyweight Equipment = "bodyweight"
)

// BodyPart identifies an injury location. An exercise that lists a body part in
// its contraindications is excluded for users with an active injury there.
type BodyPart string

const (
	LowerBack BodyPart = "lower_back"
	Knee      BodyPart = "knee"
	Shoulder  BodyPart = "shoulder"
	Elbow     BodyPart = "elbow"
	Wrist     BodyPart = "wrist"
	Hip       BodyPart = "hip"
	Ankle     BodyPart = "ankle"
	Neck      BodyPart = "neck"
)
