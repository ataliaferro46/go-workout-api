package domain

import "time"

// Workout represents a single logged training session belonging to a user.
// PlanID and PlanDayIdx are populated when the workout originated from a
// generated plan day; an ad-hoc gym session leaves them empty. Type
// discriminates between strength sessions (the default — Exercises is the
// payload) and cardio sessions (CardioSession is the payload).
type Workout struct {
	ID            string           `json:"id"`
	UserID        string           `json:"user_id"`
	Name          string           `json:"name"`
	Notes         string           `json:"notes,omitempty"`
	Type          string           `json:"type,omitempty"` // "strength" (default) | "cardio"
	PlanID        string           `json:"plan_id,omitempty"`
	PlanDayIdx    int              `json:"plan_day_idx,omitempty"`
	Exercises     []LoggedExercise `json:"exercises"`
	CardioSession *CardioSession   `json:"cardio_session,omitempty"`
	CreatedAt     time.Time        `json:"created_at"`
}

// CardioSession records the specific data for a cardio workout: what
// activity, how long, how hard, how far, how many calories burned.
// Calories come from one of:
//   - the user's Oura ring (CaloriesSource = "oura")
//   - a MET-formula estimate using the user's weight + activity (CaloriesSource = "met_formula")
//   - manual input (CaloriesSource = "" or "manual")
type CardioSession struct {
	Activity        string  `json:"activity"`         // "running", "cycling", "walking", "rowing", "swimming", "hiit", "other"
	Intensity       string  `json:"intensity"`        // "light" | "moderate" | "vigorous"
	DurationMinutes int     `json:"duration_minutes"`
	DistanceKM      float64 `json:"distance_km,omitempty"`
	AvgHR           int     `json:"avg_hr,omitempty"`
	Calories        int     `json:"calories"`
	CaloriesSource  string  `json:"calories_source,omitempty"`
	Notes           string  `json:"notes,omitempty"`
}

// LoggedExercise is a movement the user actually performed in a session.
// Sets / Reps / WeightKG (the original flat fields) summarize the
// prescription. LoggedSets is the per-set ground truth populated as the
// user logs each set in real time. TargetReps, when populated, holds a
// per-set rep target for pyramid / variable-rep schemes (e.g. 10, 8, 6).
// Prescription carries the rich planning metadata (set type, warmups,
// rest period) when the workout was started from a planned day; null for
// ad-hoc workouts.
type LoggedExercise struct {
	Name         string             `json:"name"`
	Sets         int                `json:"sets"`     // prescribed working-set count
	Reps         int                `json:"reps"`     // prescribed (mid of range)
	WeightKG     float64            `json:"weight_kg"`
	TargetReps   []int              `json:"target_reps,omitempty"`
	Prescription *ExercisePrescription `json:"prescription,omitempty"`
	LoggedSets   []LoggedSet        `json:"logged_sets,omitempty"`
}

// ExercisePrescription is the planning metadata carried over from the
// generated plan so the live workout view matches what the user saw on
// /plans: set type variants (AMRAP / drop set / 21s / superset), warmup
// ramp, and rest period.
type ExercisePrescription struct {
	SetType     string      `json:"set_type,omitempty"`
	SetTypeNote string      `json:"set_type_note,omitempty"`
	Warmups     []WarmupSet `json:"warmups,omitempty"`
	RepsLow     int         `json:"reps_low,omitempty"`
	RepsHigh    int         `json:"reps_high,omitempty"`
	RestSeconds int         `json:"rest_seconds,omitempty"`
}

// LoggedSet is one completed set with the actual reps + weight the user
// performed and the moment they marked it done. The per-set log is what
// powers progress tracking (estimated 1RM, volume/session, PR detection).
type LoggedSet struct {
	SetNumber   int       `json:"set_number"`
	Reps        int       `json:"reps"`
	WeightKG    float64   `json:"weight_kg"`
	CompletedAt time.Time `json:"completed_at"`
}
