package workout

import (
	"context"
	"strings"
	"time"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
)

// IDGenerator produces unique identifiers. It is injected so tests can use
// deterministic IDs instead of random UUIDs.
type IDGenerator func() string

// Clock returns the current time. It is injected so tests are deterministic
// and not dependent on the wall clock.
type Clock func() time.Time

// Service contains the business logic for workouts. It depends only on the
// Repository interface and two small function types, which makes it trivial
// to unit-test in isolation from HTTP and storage.
type Service struct {
	repo  Repository
	newID IDGenerator
	now   Clock
}

// NewService constructs a Service. Passing nil for newID or now selects
// production defaults (random UUIDs and the system clock).
func NewService(repo Repository, newID IDGenerator, now Clock) *Service {
	if newID == nil {
		newID = NewUUID
	}
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, newID: newID, now: now}
}

// CreateInput carries the fields needed to create a workout. Keeping it
// separate from the HTTP request type and the domain type means the
// transport and persistence layers can evolve independently.
type CreateInput struct {
	UserID    string
	Name      string
	Notes     string
	Exercises []domain.Exercise
}

// Create validates the input, assembles a domain.Workout, and persists it.
func (s *Service) Create(ctx context.Context, in CreateInput) (domain.Workout, error) {
	if err := validateCreate(in); err != nil {
		return domain.Workout{}, err
	}
	w := domain.Workout{
		ID:        s.newID(),
		UserID:    strings.TrimSpace(in.UserID),
		Name:      strings.TrimSpace(in.Name),
		Notes:     strings.TrimSpace(in.Notes),
		Exercises: in.Exercises,
		CreatedAt: s.now().UTC(),
	}
	if w.Exercises == nil {
		w.Exercises = []domain.Exercise{}
	}
	if err := s.repo.Create(ctx, w); err != nil {
		return domain.Workout{}, err
	}
	return w, nil
}

// Get returns a single workout by ID.
func (s *Service) Get(ctx context.Context, id string) (domain.Workout, error) {
	return s.repo.Get(ctx, id)
}

// ListByUser returns all workouts belonging to a user.
func (s *Service) ListByUser(ctx context.Context, userID string) ([]domain.Workout, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, &domain.ValidationError{Message: "user id is required"}
	}
	return s.repo.ListByUser(ctx, userID)
}

// Delete removes a workout by ID.
func (s *Service) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

func validateCreate(in CreateInput) error {
	if strings.TrimSpace(in.UserID) == "" {
		return &domain.ValidationError{Message: "user id is required"}
	}
	if strings.TrimSpace(in.Name) == "" {
		return &domain.ValidationError{Message: "name is required"}
	}
	for _, e := range in.Exercises {
		if strings.TrimSpace(e.Name) == "" {
			return &domain.ValidationError{Message: "exercise name is required"}
		}
		if e.Sets < 0 || e.Reps < 0 || e.WeightKG < 0 {
			return &domain.ValidationError{Message: "exercise sets, reps, and weight must be non-negative"}
		}
	}
	return nil
}
