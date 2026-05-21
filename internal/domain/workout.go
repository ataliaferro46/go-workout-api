package domain

import "time"

// Workout represents a single logged training session belonging to a user.
type Workout struct {
	ID        string           `json:"id"`
	UserID    string           `json:"user_id"`
	Name      string           `json:"name"`
	Notes     string           `json:"notes,omitempty"`
	Exercises []LoggedExercise `json:"exercises"`
	CreatedAt time.Time        `json:"created_at"`
}

// LoggedExercise is a movement the user actually performed in a session, with
// the volume they did. It is distinct from the library Exercise (exercise.go),
// which describes a movement the engine can prescribe.
type LoggedExercise struct {
	Name     string  `json:"name"`
	Sets     int     `json:"sets"`
	Reps     int     `json:"reps"`
	WeightKG float64 `json:"weight_kg"`
}
