// Package exercise owns the canonical movement library: the seed data, the
// Repository abstraction over it, the boot-time-cached Service the plan engine
// reads from, and the admin HTTP endpoints for editing it.
//
// The seed data in seed.go is the source of truth for two non-overlapping
// paths: (a) the InMemoryRepository, used for `go run`, demos, and unit tests
// without Postgres; (b) the 0003_exercises migration, which inserts the same
// rows into Postgres at first boot of the Postgres-mode binary. After that,
// the database is the source of truth — additions ship as follow-up
// migrations (and the seed list is updated for parity, but the database wins
// at runtime).
package exercise

import "github.com/ataliaferro46/go-workout-api/internal/domain"

// Library returns a fresh copy of the seed exercise list. Kept for backward
// compatibility with cmd/demo, which intentionally bypasses the Service +
// Repository layering for a zero-setup CLI demo.
//
// Production callers (cmd/api) should use Service.Snapshot() instead — that
// path reads through the Repository, so an admin-added exercise is visible
// without a binary rebuild.
func Library() []domain.Exercise {
	out := make([]domain.Exercise, len(seedExercises))
	copy(out, seedExercises)
	return out
}

// Seed returns the canonical seed list as a fresh copy. Used by
// NewInMemoryRepository so the in-memory mode boots with the same content the
// Postgres migration inserts.
func Seed() []domain.Exercise {
	out := make([]domain.Exercise, len(seedExercises))
	copy(out, seedExercises)
	return out
}
