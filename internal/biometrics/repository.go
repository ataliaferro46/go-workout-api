package biometrics

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
)

// ReadingRepository abstracts persistence for ingested biometric readings.
// The InMemory implementation is used for unit tests and dev mode; Postgres
// is the production implementation. A single contract test exercises both
// so behavior cannot drift.
type ReadingRepository interface {
	// Insert persists a reading. Implementations MUST be idempotent on
	// (provider, idempotency_key) — a duplicate insert is a no-op, not an
	// error. This is the foundation of safe webhook retries and double-
	// polling.
	Insert(ctx context.Context, r domain.Reading) error

	// LatestByUserAndKind returns the most recent reading of `kind` for
	// `userID`, or domain.ErrNotFound if there are none.
	LatestByUserAndKind(ctx context.Context, userID string, kind domain.Kind) (domain.Reading, error)

	// ListByUser returns all readings for the user, newest first.
	ListByUser(ctx context.Context, userID string) ([]domain.Reading, error)
}

// SyncStateRepository tracks per-(user, provider) watermarks for polling.
type SyncStateRepository interface {
	Get(ctx context.Context, userID, provider string) (SyncState, error)
	Set(ctx context.Context, state SyncState) error
}

// SyncState is a polling watermark. The polling daemon advances
// LastSyncedAt after each successful fetch so the next poll requests only
// events recorded after that point.
type SyncState struct {
	UserID, Provider, LastEventCursor string
	LastSyncedAt                      time.Time
}

// --- in-memory implementations -------------------------------------------

// InMemoryReadingRepository is a concurrency-safe, in-memory implementation
// of ReadingRepository. It enforces the same idempotency invariant
// (provider, idempotency_key) the Postgres UNIQUE constraint enforces.
type InMemoryReadingRepository struct {
	mu       sync.RWMutex
	readings map[string]domain.Reading      // id → reading
	dedup    map[string]map[string]struct{} // provider → set of idempotency keys
}

// NewInMemoryReadingRepository returns an empty in-memory reading repo.
func NewInMemoryReadingRepository() *InMemoryReadingRepository {
	return &InMemoryReadingRepository{
		readings: map[string]domain.Reading{},
		dedup:    map[string]map[string]struct{}{},
	}
}

func (r *InMemoryReadingRepository) Insert(_ context.Context, reading domain.Reading) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.dedup[reading.Provider]; !ok {
		r.dedup[reading.Provider] = map[string]struct{}{}
	}
	if _, dup := r.dedup[reading.Provider][reading.IdempotencyKey]; dup {
		return nil // idempotent no-op
	}
	r.dedup[reading.Provider][reading.IdempotencyKey] = struct{}{}
	r.readings[reading.ID] = reading
	return nil
}

func (r *InMemoryReadingRepository) LatestByUserAndKind(_ context.Context, userID string, kind domain.Kind) (domain.Reading, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var best domain.Reading
	found := false
	for _, reading := range r.readings {
		if reading.UserID != userID || reading.Kind != kind {
			continue
		}
		if !found || reading.RecordedAt.After(best.RecordedAt) {
			best = reading
			found = true
		}
	}
	if !found {
		return domain.Reading{}, domain.ErrNotFound
	}
	return best, nil
}

func (r *InMemoryReadingRepository) ListByUser(_ context.Context, userID string) ([]domain.Reading, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.Reading, 0)
	for _, reading := range r.readings {
		if reading.UserID == userID {
			out = append(out, reading)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].RecordedAt.Equal(out[j].RecordedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].RecordedAt.After(out[j].RecordedAt)
	})
	return out, nil
}

// InMemorySyncStateRepository is the in-memory implementation of
// SyncStateRepository.
type InMemorySyncStateRepository struct {
	mu     sync.RWMutex
	states map[string]SyncState // key: userID + "\x00" + provider
}

// NewInMemorySyncStateRepository returns an empty in-memory sync-state repo.
func NewInMemorySyncStateRepository() *InMemorySyncStateRepository {
	return &InMemorySyncStateRepository{states: map[string]SyncState{}}
}

// ErrSyncStateNotFound signals no watermark exists for the pair — the
// polling daemon treats this as "never synced; fetch from epoch."
var ErrSyncStateNotFound = errors.New("sync state not found")

func syncKey(userID, provider string) string { return userID + "\x00" + provider }

func (r *InMemorySyncStateRepository) Get(_ context.Context, userID, provider string) (SyncState, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.states[syncKey(userID, provider)]
	if !ok {
		return SyncState{}, ErrSyncStateNotFound
	}
	return s, nil
}

func (r *InMemorySyncStateRepository) Set(_ context.Context, state SyncState) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.states[syncKey(state.UserID, state.Provider)] = state
	return nil
}
