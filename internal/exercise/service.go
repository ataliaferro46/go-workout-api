package exercise

import (
	"context"
	"strings"
	"sync/atomic"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
)

// Service owns the read-through and write-through paths for the exercise
// library. It also maintains a boot-time-cached snapshot that the plan engine
// reads on every Generate call without hitting the database — admin writes
// refresh the snapshot after they commit.
//
// The cache is a *[]domain.Exercise stored atomically. Readers do a single
// atomic load; writers (admin endpoints, post-mutation) do a single atomic
// store. No mutex on the read path.
type Service struct {
	repo  Repository
	cache atomic.Pointer[[]domain.Exercise]
}

// NewService constructs a Service. Call LoadSnapshot once at boot before
// passing the result of Snapshot to the plan service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// LoadSnapshot fetches the library from the repository and replaces the
// in-memory snapshot. Safe to call concurrently with Snapshot — the swap is
// atomic — and idempotent on success.
func (s *Service) LoadSnapshot(ctx context.Context) error {
	list, err := s.repo.List(ctx)
	if err != nil {
		return err
	}
	s.cache.Store(&list)
	return nil
}

// Snapshot returns the cached library. Callers MUST NOT mutate the returned
// slice — it is shared with every other reader. The plan engine treats it as
// immutable; defensive-copying it on every Generate would allocate a 100-
// element slice unnecessarily on the hot path.
//
// Returns nil if LoadSnapshot has not been called. main.go calls it at boot,
// before the HTTP server starts serving.
func (s *Service) Snapshot() []domain.Exercise {
	p := s.cache.Load()
	if p == nil {
		return nil
	}
	return *p
}

// Get reads through the repository — admin write paths don't get to see
// stale data, and the cost is one DB round trip per request.
func (s *Service) Get(ctx context.Context, id string) (domain.Exercise, error) {
	return s.repo.Get(ctx, id)
}

// List returns the cached snapshot directly. It's the read-heavy path —
// admin tools, the public GET /v1/exercises endpoint, the plan engine.
func (s *Service) List(ctx context.Context) ([]domain.Exercise, error) {
	snap := s.Snapshot()
	if snap == nil {
		// Cache hasn't been loaded yet — fall through to the repo so the
		// public List endpoint doesn't 500 during a slow boot.
		return s.repo.List(ctx)
	}
	return snap, nil
}

// Create validates the input, persists, and refreshes the cached snapshot.
func (s *Service) Create(ctx context.Context, e domain.Exercise) (domain.Exercise, error) {
	if err := validateExercise(e); err != nil {
		return domain.Exercise{}, err
	}
	if err := s.repo.Create(ctx, e); err != nil {
		return domain.Exercise{}, err
	}
	if err := s.LoadSnapshot(ctx); err != nil {
		// We persisted successfully — the failure to refresh is observable
		// (next Snapshot read will be stale) but we don't lie about Create's
		// success. Caller / operator handles it.
		return e, err
	}
	return e, nil
}

// Update validates the input, persists the change, and refreshes the cache.
func (s *Service) Update(ctx context.Context, e domain.Exercise) (domain.Exercise, error) {
	if err := validateExercise(e); err != nil {
		return domain.Exercise{}, err
	}
	if err := s.repo.Update(ctx, e); err != nil {
		return domain.Exercise{}, err
	}
	if err := s.LoadSnapshot(ctx); err != nil {
		return e, err
	}
	return e, nil
}

// Delete removes the exercise and refreshes the cache.
func (s *Service) Delete(ctx context.Context, id string) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	return s.LoadSnapshot(ctx)
}

// validateExercise enforces the same invariants as the library_test.go
// well-formed test — every public mutation must produce a well-formed row.
func validateExercise(e domain.Exercise) error {
	if strings.TrimSpace(e.ID) == "" {
		return &domain.ValidationError{Message: "id is required"}
	}
	if strings.TrimSpace(e.Name) == "" {
		return &domain.ValidationError{Message: "name is required"}
	}
	if e.PrimaryMuscle == "" {
		return &domain.ValidationError{Message: "primary_muscle is required"}
	}
	if e.Pattern == "" {
		return &domain.ValidationError{Message: "pattern is required"}
	}
	if len(e.RequiredEquipment) == 0 {
		return &domain.ValidationError{Message: "required_equipment must list at least one item (use 'bodyweight' if none)"}
	}
	if !e.MinLevel.Valid() {
		return &domain.ValidationError{Message: "min_level must be beginner, intermediate, or advanced"}
	}
	return nil
}
