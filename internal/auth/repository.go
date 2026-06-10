package auth

import (
	"context"
	"strings"
	"sync"
	"time"
)

// UserRepository persists user accounts. Email lookups are case-insensitive.
type UserRepository interface {
	Create(ctx context.Context, c UserCredentials) error
	GetByID(ctx context.Context, id string) (UserCredentials, error)
	GetByEmail(ctx context.Context, email string) (UserCredentials, error)
	MarkVerified(ctx context.Context, userID string, when time.Time) error
	UpdateProfile(ctx context.Context, userID string, heightCM *int, weightKG *float64, birth *time.Time, sex *string, now time.Time) error
	UpdateBodyFat(ctx context.Context, userID string, bf *float64, now time.Time) error
	UpdateWorkoutTime(ctx context.Context, userID, workoutTime string, now time.Time) error
}

// SessionRepository persists session-token hashes. Plaintext tokens never
// touch the repository.
type SessionRepository interface {
	Insert(ctx context.Context, s Session) error
	GetByTokenHash(ctx context.Context, tokenHash string) (Session, error)
	Delete(ctx context.Context, tokenHash string) error
	DeleteAllForUser(ctx context.Context, userID string) error
}

// VerificationRepository persists email-verification token hashes.
type VerificationRepository interface {
	Insert(ctx context.Context, v Verification) error
	Consume(ctx context.Context, tokenHash string, when time.Time) (Verification, error)
}

// --- in-memory implementations -----------------------------------------

type InMemoryUserRepository struct {
	mu      sync.RWMutex
	byID    map[string]UserCredentials
	byEmail map[string]string // lower(email) → id
}

func NewInMemoryUserRepository() *InMemoryUserRepository {
	return &InMemoryUserRepository{
		byID:    map[string]UserCredentials{},
		byEmail: map[string]string{},
	}
}

func (r *InMemoryUserRepository) Create(_ context.Context, c UserCredentials) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := strings.ToLower(c.User.Email)
	if _, taken := r.byEmail[key]; taken {
		return ErrEmailTaken
	}
	r.byID[c.User.ID] = c
	r.byEmail[key] = c.User.ID
	return nil
}

func (r *InMemoryUserRepository) GetByID(_ context.Context, id string) (UserCredentials, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.byID[id]
	if !ok {
		return UserCredentials{}, ErrUserNotFound
	}
	return c, nil
}

func (r *InMemoryUserRepository) GetByEmail(_ context.Context, email string) (UserCredentials, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.byEmail[strings.ToLower(email)]
	if !ok {
		return UserCredentials{}, ErrUserNotFound
	}
	c := r.byID[id]
	return c, nil
}

func (r *InMemoryUserRepository) MarkVerified(_ context.Context, userID string, when time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.byID[userID]
	if !ok {
		return ErrUserNotFound
	}
	c.User.EmailVerifiedAt = &when
	c.User.UpdatedAt = when
	r.byID[userID] = c
	return nil
}

func (r *InMemoryUserRepository) UpdateWorkoutTime(_ context.Context, userID, wt string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.byID[userID]
	if !ok {
		return ErrUserNotFound
	}
	c.User.WorkoutTime = wt
	c.User.UpdatedAt = now
	r.byID[userID] = c
	return nil
}

func (r *InMemoryUserRepository) UpdateBodyFat(_ context.Context, userID string, bf *float64, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.byID[userID]
	if !ok {
		return ErrUserNotFound
	}
	c.User.BodyFatPercentage = bf
	c.User.UpdatedAt = now
	r.byID[userID] = c
	return nil
}

func (r *InMemoryUserRepository) UpdateProfile(_ context.Context, userID string, heightCM *int, weightKG *float64, birth *time.Time, sex *string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.byID[userID]
	if !ok {
		return ErrUserNotFound
	}
	if heightCM != nil {
		c.User.HeightCM = heightCM
	}
	if weightKG != nil {
		c.User.WeightKG = weightKG
	}
	if birth != nil {
		c.User.BirthDate = birth
	}
	if sex != nil {
		c.User.Sex = *sex
	}
	c.User.UpdatedAt = now
	r.byID[userID] = c
	return nil
}

type InMemorySessionRepository struct {
	mu       sync.RWMutex
	sessions map[string]Session // token hash → session
}

func NewInMemorySessionRepository() *InMemorySessionRepository {
	return &InMemorySessionRepository{sessions: map[string]Session{}}
}

func (r *InMemorySessionRepository) Insert(_ context.Context, s Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[s.TokenHash] = s
	return nil
}

func (r *InMemorySessionRepository) GetByTokenHash(_ context.Context, tokenHash string) (Session, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.sessions[tokenHash]
	if !ok {
		return Session{}, ErrSessionExpired
	}
	if time.Now().After(s.ExpiresAt) {
		return Session{}, ErrSessionExpired
	}
	return s, nil
}

func (r *InMemorySessionRepository) Delete(_ context.Context, tokenHash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, tokenHash)
	return nil
}

func (r *InMemorySessionRepository) DeleteAllForUser(_ context.Context, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for hash, s := range r.sessions {
		if s.UserID == userID {
			delete(r.sessions, hash)
		}
	}
	return nil
}

type InMemoryVerificationRepository struct {
	mu     sync.RWMutex
	tokens map[string]Verification
}

func NewInMemoryVerificationRepository() *InMemoryVerificationRepository {
	return &InMemoryVerificationRepository{tokens: map[string]Verification{}}
}

func (r *InMemoryVerificationRepository) Insert(_ context.Context, v Verification) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tokens[v.TokenHash] = v
	return nil
}

func (r *InMemoryVerificationRepository) Consume(_ context.Context, tokenHash string, when time.Time) (Verification, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.tokens[tokenHash]
	if !ok {
		return Verification{}, ErrTokenInvalid
	}
	if v.ConsumedAt != nil || when.After(v.ExpiresAt) {
		return Verification{}, ErrTokenInvalid
	}
	v.ConsumedAt = &when
	r.tokens[tokenHash] = v
	return v, nil
}
