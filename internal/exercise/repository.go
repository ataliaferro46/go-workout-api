package exercise

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
)

// ErrDuplicateName is returned when an Insert or Update would violate the
// uniqueness of the exercise name. It wraps domain.ErrConflict so the
// transport layer's generic conflict mapping (httpx.Error → 409) works
// without httpx needing to know about exercise-specific sentinels.
var ErrDuplicateName = fmt.Errorf("%w: exercise with that name already exists", domain.ErrConflict)

// Repository abstracts persistence for the exercise library. The Service
// depends on the interface; production wires in PostgresRepository, tests and
// the in-memory mode wire in InMemoryRepository.
//
// Note: there is no ListByUser-style scoping here because the library is
// global (every user shares the same set of canonical movements). A future
// per-user custom-exercise feature would add a new method with a user filter,
// not replace the existing global one.
type Repository interface {
	Create(ctx context.Context, e domain.Exercise) error
	Get(ctx context.Context, id string) (domain.Exercise, error)
	Update(ctx context.Context, e domain.Exercise) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context) ([]domain.Exercise, error)
	ListByPattern(ctx context.Context, pattern domain.MovementPattern) ([]domain.Exercise, error)
}

// InMemoryRepository is a concurrency-safe, in-memory Repository. It is
// pre-seeded at construction with the canonical seed list so in-memory mode
// starts with the same library Postgres mode has after migration. The
// RWMutex matches the access pattern of the Postgres pool: many concurrent
// reads, serialized writes.
type InMemoryRepository struct {
	mu        sync.RWMutex
	exercises map[string]domain.Exercise
	// nameIdx enforces the same uniqueness on name that Postgres does via the
	// UNIQUE constraint. Insert and Update consult it so both implementations
	// return ErrDuplicateName at the same boundary.
	nameIdx map[string]string // name → id
}

// NewInMemoryRepository returns a Repository pre-seeded with the canonical
// seed list. Pass exercise.Seed() at construction in main and in tests that
// need the standard fixture; pass nil for an empty repo (useful for repo
// contract tests that start empty).
func NewInMemoryRepository(seed []domain.Exercise) *InMemoryRepository {
	r := &InMemoryRepository{
		exercises: make(map[string]domain.Exercise, len(seed)),
		nameIdx:   make(map[string]string, len(seed)),
	}
	for _, e := range seed {
		r.exercises[e.ID] = e
		r.nameIdx[e.Name] = e.ID
	}
	return r
}

func (r *InMemoryRepository) Create(ctx context.Context, e domain.Exercise) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.nameIdx[e.Name]; exists {
		return ErrDuplicateName
	}
	r.exercises[e.ID] = e
	r.nameIdx[e.Name] = e.ID
	return nil
}

func (r *InMemoryRepository) Get(ctx context.Context, id string) (domain.Exercise, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.exercises[id]
	if !ok {
		return domain.Exercise{}, domain.ErrNotFound
	}
	return e, nil
}

// Update replaces the exercise atomically. If the name changed, the name
// index is checked for collision with any other row first.
func (r *InMemoryRepository) Update(ctx context.Context, e domain.Exercise) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	old, ok := r.exercises[e.ID]
	if !ok {
		return domain.ErrNotFound
	}
	if old.Name != e.Name {
		if existingID, taken := r.nameIdx[e.Name]; taken && existingID != e.ID {
			return ErrDuplicateName
		}
		delete(r.nameIdx, old.Name)
		r.nameIdx[e.Name] = e.ID
	}
	r.exercises[e.ID] = e
	return nil
}

func (r *InMemoryRepository) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.exercises[id]
	if !ok {
		return domain.ErrNotFound
	}
	delete(r.exercises, id)
	delete(r.nameIdx, e.Name)
	return nil
}

// List returns every exercise, sorted by ID for deterministic order. Both
// the contract test and the Postgres implementation rely on this ordering.
func (r *InMemoryRepository) List(ctx context.Context) ([]domain.Exercise, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.Exercise, 0, len(r.exercises))
	for _, e := range r.exercises {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// ListByPattern returns every exercise of the given pattern, sorted by ID.
func (r *InMemoryRepository) ListByPattern(ctx context.Context, pattern domain.MovementPattern) ([]domain.Exercise, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.Exercise, 0)
	for _, e := range r.exercises {
		if e.Pattern == pattern {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
