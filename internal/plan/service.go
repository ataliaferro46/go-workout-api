package plan

import (
	"context"
	"strings"
	"time"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
)

// IDGenerator produces unique identifiers. Injected so tests can use
// deterministic IDs instead of random UUIDs.
type IDGenerator func() string

// Clock returns the current time. Injected so tests are deterministic and not
// dependent on the wall clock.
type Clock func() time.Time

// LibrarySource returns the current exercise library snapshot. Plan.Service
// holds a LibrarySource (not a static slice) so admin edits to the exercise
// library take effect on the next Generate call without a process restart.
// In production, main.go passes exercise.Service.Snapshot; in tests, a
// closure over a static fixture.
type LibrarySource func() []domain.Exercise

// RecoverySource returns the user's latest recovery-shaped reading
// normalized to 0.0–1.0. Returns false if no fresh recovery is available;
// the plan service treats absence as "no adjustment." Production wires a
// biometrics.Service adapter here; tests can pass nil to disable recovery-
// awareness entirely.
type RecoverySource interface {
	LatestRecovery(ctx context.Context, userID string) (value float64, fresh bool, err error)
}

// Service owns the lifecycle of generated workout plans: it wraps the
// Generator with persistence, ID generation, and timestamping. The Handler
// depends on the Service; the Service depends on the Repository interface
// and the LibrarySource function.
type Service struct {
	library  LibrarySource
	repo     Repository
	recovery RecoverySource
	newID    IDGenerator
	now      Clock
}

// NewService constructs a Service. Passing nil for newID or now selects
// production defaults (random UUIDs and the system clock). recovery may be
// nil — recovery-aware generation is disabled in that case.
func NewService(library LibrarySource, repo Repository, recovery RecoverySource, newID IDGenerator, now Clock) *Service {
	if newID == nil {
		newID = NewUUID
	}
	if now == nil {
		now = time.Now
	}
	return &Service{library: library, repo: repo, recovery: recovery, newID: newID, now: now}
}

// Create runs the generation engine for the given user and request, stamps
// persistence metadata (ID, UserID, CreatedAt), and persists the plan via the
// repository. The seed makes generation reproducible — pass time.Now().UnixNano()
// for variety, or a fixed value for determinism. When recoveryAware is true
// and a fresh recovery reading exists, the engine biases scoring away from
// high-intensity compounds (ADR-051) and emits a warning explaining the
// adjustment.
func (s *Service) Create(ctx context.Context, userID string, req domain.GenerateRequest, seed int64, recoveryAware bool) (domain.WorkoutPlan, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return domain.WorkoutPlan{}, &domain.ValidationError{Message: "user id is required"}
	}

	// Recovery-aware path: populate req.RecoveryHint from biometrics so the
	// Generator's scoring downstream picks it up. Failure to read recovery
	// is silent — we proceed without a hint rather than failing the plan
	// generation.
	if recoveryAware && s.recovery != nil {
		if value, fresh, err := s.recovery.LatestRecovery(ctx, userID); err == nil && fresh {
			req.RecoveryHint = &value
		}
	}

	// Build a fresh Generator per call so concurrent requests do not share
	// mutable RNG state (mirrors the rationale in ADR-013). The library is
	// read at call time from the LibrarySource so admin edits take effect
	// immediately.
	gen := NewGenerator(s.library(), seed)
	p, err := gen.Generate(req)
	if err != nil {
		return domain.WorkoutPlan{}, err
	}

	p.ID = s.newID()
	p.UserID = userID
	p.CreatedAt = s.now().UTC()
	if p.Warnings == nil {
		p.Warnings = []string{}
	}

	if err := s.repo.Create(ctx, p); err != nil {
		return domain.WorkoutPlan{}, err
	}
	return p, nil
}

// Get returns the persisted plan with the given ID.
func (s *Service) Get(ctx context.Context, id string) (domain.WorkoutPlan, error) {
	return s.repo.Get(ctx, id)
}

// ListByUser returns all persisted plans for the given user.
func (s *Service) ListByUser(ctx context.Context, userID string) ([]domain.WorkoutPlan, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, &domain.ValidationError{Message: "user id is required"}
	}
	return s.repo.ListByUser(ctx, userID)
}

// Delete removes a persisted plan by ID.
func (s *Service) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}
