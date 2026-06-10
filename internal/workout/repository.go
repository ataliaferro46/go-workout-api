package workout

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
)

// Repository abstracts persistence for logged workouts so the service is
// storage-agnostic. The in-memory implementation below is used for tests and
// for running the service without a database; a Postgres implementation can
// satisfy the same interface.
type Repository interface {
	Create(ctx context.Context, w domain.Workout) error
	Get(ctx context.Context, id string) (domain.Workout, error)
	ListByUser(ctx context.Context, userID string) ([]domain.Workout, error)
	Delete(ctx context.Context, id string) error
	// LogSet upserts a single completed set. Idempotent on
	// (workoutID, exercisePosition, setNumber).
	LogSet(ctx context.Context, workoutID string, exercisePosition, setNumber, reps int, weightKG float64, completedAt time.Time) error
	// LastSetsForExercise returns the most recent logged sets the user
	// has recorded for the named exercise (case-insensitive). The second
	// return value is the timestamp of the latest set. Both zero when no
	// match exists — used by the live workout page's "last time" hint.
	LastSetsForExercise(ctx context.Context, userID, exerciseName string) ([]domain.LoggedSet, time.Time, error)
	// UpdateExercise patches the prescription of one exercise inside a
	// workout (sets / reps / weight / target reps / set type). Used by
	// the inline editor.
	UpdateExercise(ctx context.Context, workoutID string, position int, sets, reps int, weightKG float64, targetReps []int, prescription *domain.ExercisePrescription) error
}

// InMemoryRepository is a concurrency-safe, in-memory Repository. The RWMutex
// allows concurrent reads while serializing writes — the access pattern a real
// datastore connection pool would exhibit.
type InMemoryRepository struct {
	mu       sync.RWMutex
	workouts map[string]domain.Workout
}

// NewInMemoryRepository returns an empty in-memory repository.
func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{workouts: make(map[string]domain.Workout)}
}

// Create stores a workout. It assumes the ID is already set by the caller.
func (r *InMemoryRepository) Create(ctx context.Context, w domain.Workout) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.workouts[w.ID] = w
	return nil
}

// Get returns the workout with the given ID or domain.ErrNotFound.
func (r *InMemoryRepository) Get(ctx context.Context, id string) (domain.Workout, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	w, ok := r.workouts[id]
	if !ok {
		return domain.Workout{}, domain.ErrNotFound
	}
	return w, nil
}

// ListByUser returns a user's workouts, newest first. The result is always a
// non-nil slice so callers (and JSON output) get [] rather than null.
func (r *InMemoryRepository) ListByUser(ctx context.Context, userID string) ([]domain.Workout, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.Workout, 0)
	for _, w := range r.workouts {
		if w.UserID == userID {
			out = append(out, w)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

// LogSet inserts/upserts a per-set log entry on the in-memory workout.
func (r *InMemoryRepository) LogSet(ctx context.Context, workoutID string, exercisePosition, setNumber, reps int, weightKG float64, completedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	w, ok := r.workouts[workoutID]
	if !ok {
		return domain.ErrNotFound
	}
	if exercisePosition < 0 || exercisePosition >= len(w.Exercises) {
		return domain.ErrNotFound
	}
	logs := w.Exercises[exercisePosition].LoggedSets
	replaced := false
	for i := range logs {
		if logs[i].SetNumber == setNumber {
			logs[i] = domain.LoggedSet{SetNumber: setNumber, Reps: reps, WeightKG: weightKG, CompletedAt: completedAt}
			replaced = true
			break
		}
	}
	if !replaced {
		logs = append(logs, domain.LoggedSet{SetNumber: setNumber, Reps: reps, WeightKG: weightKG, CompletedAt: completedAt})
		sort.Slice(logs, func(i, j int) bool { return logs[i].SetNumber < logs[j].SetNumber })
	}
	w.Exercises[exercisePosition].LoggedSets = logs
	r.workouts[workoutID] = w
	return nil
}

// LastSetsForExercise scans the in-memory store newest-first and returns
// the first match's logged sets.
func (r *InMemoryRepository) LastSetsForExercise(ctx context.Context, userID, exerciseName string) ([]domain.LoggedSet, time.Time, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	candidates := make([]domain.Workout, 0)
	for _, w := range r.workouts {
		if w.UserID == userID {
			candidates = append(candidates, w)
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].CreatedAt.After(candidates[j].CreatedAt) })
	for _, w := range candidates {
		for _, ex := range w.Exercises {
			if eqInsensitive(ex.Name, exerciseName) && len(ex.LoggedSets) > 0 {
				return ex.LoggedSets, ex.LoggedSets[len(ex.LoggedSets)-1].CompletedAt, nil
			}
		}
	}
	return nil, time.Time{}, nil
}

func eqInsensitive(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 32
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// UpdateExercise patches the in-memory exercise prescription at position.
func (r *InMemoryRepository) UpdateExercise(ctx context.Context, workoutID string, position int, sets, reps int, weightKG float64, targetReps []int, prescription *domain.ExercisePrescription) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	w, ok := r.workouts[workoutID]
	if !ok {
		return domain.ErrNotFound
	}
	if position < 0 || position >= len(w.Exercises) {
		return domain.ErrNotFound
	}
	w.Exercises[position].Sets = sets
	w.Exercises[position].Reps = reps
	w.Exercises[position].WeightKG = weightKG
	w.Exercises[position].TargetReps = targetReps
	w.Exercises[position].Prescription = prescription
	r.workouts[workoutID] = w
	return nil
}

// Delete removes a workout by ID or returns domain.ErrNotFound.
func (r *InMemoryRepository) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.workouts[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.workouts, id)
	return nil
}
