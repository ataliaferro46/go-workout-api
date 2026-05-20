// Package domain holds the core types and business invariants for the
// application. It depends on nothing else in the codebase, which keeps the
// dependency graph acyclic: every other package may import domain, but
// domain imports no internal package.
package domain

import "time"

// Workout represents a single training session belonging to a user.
type Workout struct {
	ID        string     `json:"id"`
	UserID    string     `json:"user_id"`
	Name      string     `json:"name"`
	Notes     string     `json:"notes,omitempty"`
	Exercises []Exercise `json:"exercises"`
	CreatedAt time.Time  `json:"created_at"`
}

// Exercise is a single movement performed within a workout.
type Exercise struct {
	Name     string  `json:"name"`
	Sets     int     `json:"sets"`
	Reps     int     `json:"reps"`
	WeightKG float64 `json:"weight_kg"`
}
