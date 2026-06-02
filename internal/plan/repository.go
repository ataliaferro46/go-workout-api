package plan

import (
	"context"
	"sort"
	"sync"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
)

// Repository abstracts persistence for generated workout plans. The service
// depends on the interface; production wires in PostgresRepository, tests and
// the in-memory mode wire in InMemoryRepository.
type Repository interface {
	Create(ctx context.Context, p domain.WorkoutPlan) error
	Get(ctx context.Context, id string) (domain.WorkoutPlan, error)
	ListByUser(ctx context.Context, userID string) ([]domain.WorkoutPlan, error)
	Delete(ctx context.Context, id string) error
}

// InMemoryRepository is a concurrency-safe, in-memory Repository for plans.
// The shape mirrors workout.InMemoryRepository so reviewers see the same
// pattern in both feature packages.
type InMemoryRepository struct {
	mu    sync.RWMutex
	plans map[string]domain.WorkoutPlan
}

// NewInMemoryRepository returns an empty in-memory plan repository.
func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{plans: make(map[string]domain.WorkoutPlan)}
}

// Create stores a plan. The caller is expected to have set ID, UserID, and
// CreatedAt; the service layer does this.
func (r *InMemoryRepository) Create(ctx context.Context, p domain.WorkoutPlan) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.plans[p.ID] = p
	return nil
}

// Get returns the plan with the given ID or domain.ErrNotFound.
func (r *InMemoryRepository) Get(ctx context.Context, id string) (domain.WorkoutPlan, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.plans[id]
	if !ok {
		return domain.WorkoutPlan{}, domain.ErrNotFound
	}
	return p, nil
}

// ListByUser returns a user's plans, newest first. The result is always a
// non-nil slice so JSON output is [] rather than null.
func (r *InMemoryRepository) ListByUser(ctx context.Context, userID string) ([]domain.WorkoutPlan, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.WorkoutPlan, 0)
	for _, p := range r.plans {
		if p.UserID == userID {
			out = append(out, p)
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

// Delete removes a plan by ID or returns domain.ErrNotFound.
func (r *InMemoryRepository) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.plans[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.plans, id)
	return nil
}
