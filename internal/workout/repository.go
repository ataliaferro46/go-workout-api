package workout

import (
	"context"
	"sort"
	"sync"

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
