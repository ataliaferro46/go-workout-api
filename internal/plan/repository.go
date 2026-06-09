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

	// UpdateExercise replaces a single (plan_id, day_idx, order_idx) row's
	// exercise + warmups while preserving sets / reps / rest. Used by the
	// "swap exercise" flow on the plan view — the user keeps the day, the
	// slot's volume prescription, and only the movement changes.
	UpdateExercise(ctx context.Context, planID string, dayIdx, orderIdx int, ex domain.Exercise, warmups []domain.WarmupSet) error

	// ReorderDays remaps plan_days.day_idx (and the matching plan_exercises
	// rows) so the user can rearrange the week without regenerating. mapping
	// is old day_idx → new day_idx; every existing day must appear as a key
	// and the values must be a permutation of the keys.
	ReorderDays(ctx context.Context, planID string, mapping map[int]int) error
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

// ReorderDays remaps the day indices on the given plan.
func (r *InMemoryRepository) ReorderDays(ctx context.Context, planID string, mapping map[int]int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.plans[planID]
	if !ok {
		return domain.ErrNotFound
	}
	for i := range p.Days {
		if newIdx, ok := mapping[p.Days[i].Index]; ok {
			p.Days[i].Index = newIdx
		}
	}
	// Sort by new index so the slice order matches.
	sort.Slice(p.Days, func(i, j int) bool { return p.Days[i].Index < p.Days[j].Index })
	r.plans[planID] = p
	return nil
}

// UpdateExercise replaces the exercise + warmups at (dayIdx, orderIdx) in
// the given plan. Sets / reps / rest are preserved.
func (r *InMemoryRepository) UpdateExercise(ctx context.Context, planID string, dayIdx, orderIdx int, ex domain.Exercise, warmups []domain.WarmupSet) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.plans[planID]
	if !ok {
		return domain.ErrNotFound
	}
	for di, day := range p.Days {
		if day.Index != dayIdx {
			continue
		}
		for ei, pe := range day.Exercises {
			if pe.Order != orderIdx {
				continue
			}
			p.Days[di].Exercises[ei].Exercise = ex
			p.Days[di].Exercises[ei].Warmups = warmups
			r.plans[planID] = p
			return nil
		}
	}
	return domain.ErrNotFound
}
