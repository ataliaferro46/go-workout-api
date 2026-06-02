package domain

import "time"

// WorkoutPlan is the engine's output: a full training week, plus persistence
// metadata. ID / UserID / CreatedAt are populated by the plan.Service when a
// plan is persisted; the demo binary leaves them as zero values, which is why
// they are tagged omitempty so the wire format stays clean in both cases.
type WorkoutPlan struct {
	ID          string          `json:"id,omitempty"`
	UserID      string          `json:"user_id,omitempty"`
	Goal        Goal            `json:"goal"`
	Experience  ExperienceLevel `json:"experience"`
	DaysPerWeek int             `json:"days_per_week"`
	Split       string          `json:"split"`
	Days        []PlanDay       `json:"days"`
	Warnings    []string        `json:"warnings,omitempty"`
	CreatedAt   time.Time       `json:"created_at,omitempty"`
}

// PlanDay is a single training day within a plan.
type PlanDay struct {
	Index     int            `json:"index"`
	Name      string         `json:"name"`
	Exercises []PlanExercise `json:"exercises"`
}

// PlanExercise is a prescribed exercise: the movement plus its dosage.
type PlanExercise struct {
	Exercise    Exercise `json:"exercise"`
	Order       int      `json:"order"`
	Sets        int      `json:"sets"`
	RepsLow     int      `json:"reps_low"`
	RepsHigh    int      `json:"reps_high"`
	RestSeconds int      `json:"rest_seconds"`
}
