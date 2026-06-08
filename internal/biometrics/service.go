package biometrics

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
)

// ErrInvalidState is returned by HandleCallback when the state token is
// missing, expired, or unknown.
var ErrInvalidState = errors.New("invalid or expired oauth state")

// IDGenerator produces unique reading IDs. Injected for tests.
type IDGenerator func() string

// Clock returns the current time. Injected for tests.
type Clock func() time.Time

// Service owns the biometrics surface: OAuth connect/disconnect, reading
// lookup, and (driven by the polling daemon) ingestion via Provider.
type Service struct {
	registry  Registry
	tokens    *TokenStore
	readings  ReadingRepository
	syncState SyncStateRepository
	newID     IDGenerator
	now       Clock
	logger    *slog.Logger

	// OAuth state tokens. Short-lived (10 minute TTL) and in-memory only;
	// adequate for single-process. Scaled deployments would move this to
	// Redis or a Postgres table — documented as a follow-up in the spec.
	stateMu sync.Mutex
	states  map[string]oauthState
}

type oauthState struct {
	userID    string
	provider  string
	createdAt time.Time
}

// NewService constructs a Service.
func NewService(
	registry Registry,
	tokens *TokenStore,
	readings ReadingRepository,
	syncState SyncStateRepository,
	newID IDGenerator,
	now Clock,
	logger *slog.Logger,
) *Service {
	if newID == nil {
		newID = randomID
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{
		registry:  registry,
		tokens:    tokens,
		readings:  readings,
		syncState: syncState,
		newID:     newID,
		now:       now,
		logger:    logger,
		states:    make(map[string]oauthState),
	}
}

// Connect begins an OAuth flow for the given user and provider. It returns
// the URL the user must visit to authorize, plus the state token the
// callback handler will verify.
func (s *Service) Connect(ctx context.Context, userID, providerName string) (authURL, state string, err error) {
	p, err := s.registry.Lookup(providerName)
	if err != nil {
		return "", "", err
	}
	state = randomID()
	s.stateMu.Lock()
	s.gcStatesLocked()
	s.states[state] = oauthState{userID: userID, provider: providerName, createdAt: s.now()}
	s.stateMu.Unlock()
	return p.AuthURL(state), state, nil
}

// HandleCallback completes an OAuth flow: looks up the state, validates,
// exchanges the code, persists the encrypted token, and seeds the sync
// watermark so the next polling cycle ingests fresh data.
func (s *Service) HandleCallback(ctx context.Context, state, code string) error {
	s.stateMu.Lock()
	saved, ok := s.states[state]
	if ok {
		delete(s.states, state)
	}
	s.stateMu.Unlock()
	if !ok || s.now().Sub(saved.createdAt) > 10*time.Minute {
		return ErrInvalidState
	}

	p, err := s.registry.Lookup(saved.provider)
	if err != nil {
		return err
	}
	tok, err := p.ExchangeCode(ctx, code)
	if err != nil {
		return fmt.Errorf("exchange code: %w", err)
	}
	if err := s.tokens.Save(ctx, saved.userID, saved.provider, tok); err != nil {
		return fmt.Errorf("save token: %w", err)
	}
	if err := s.syncState.Set(ctx, SyncState{
		UserID:       saved.userID,
		Provider:     saved.provider,
		LastSyncedAt: s.now().Add(-7 * 24 * time.Hour), // bootstrap: pull last week
	}); err != nil {
		return fmt.Errorf("seed sync state: %w", err)
	}
	return nil
}

// Disconnect deletes the user's token for the provider. Subsequent polls
// will skip this (user, provider) pair.
func (s *Service) Disconnect(ctx context.Context, userID, provider string) error {
	if err := s.tokens.Delete(ctx, userID, provider); err != nil && err != ErrTokenNotFound {
		return err
	}
	return nil
}

// LatestForUser returns the most recent reading per kind for the user. Used
// by GET /v1/biometrics/latest and by the recovery-aware plan path.
func (s *Service) LatestForUser(ctx context.Context, userID string) (map[domain.Kind]domain.Reading, error) {
	out := make(map[domain.Kind]domain.Reading)
	for _, k := range []domain.Kind{
		domain.KindRecovery, domain.KindStrain,
		domain.KindSleepScore, domain.KindSleepMinutes,
		domain.KindReadiness,
	} {
		r, err := s.readings.LatestByUserAndKind(ctx, userID, k)
		if err == nil {
			out[k] = r
		}
	}
	return out, nil
}

// PlanRecoveryAdapter adapts biometrics.Service to the plan.RecoverySource
// interface so the plan package depends only on a tiny inbound interface
// rather than importing biometrics.
type PlanRecoveryAdapter struct {
	Svc          *Service
	FreshnessTTL time.Duration // default 24h: readings older than this are returned but flagged not-fresh
}

// NewPlanRecoveryAdapter returns an adapter with sensible defaults.
func NewPlanRecoveryAdapter(svc *Service) *PlanRecoveryAdapter {
	return &PlanRecoveryAdapter{Svc: svc, FreshnessTTL: 24 * time.Hour}
}

// LatestRecovery implements plan.RecoverySource. It returns the most recent
// recovery reading; fresh=false when no reading exists or it's older than
// FreshnessTTL (the plan service treats not-fresh as "skip the adjustment").
func (a *PlanRecoveryAdapter) LatestRecovery(ctx context.Context, userID string) (float64, bool, error) {
	r, err := a.Svc.LatestRecovery(ctx, userID)
	if errors.Is(err, domain.ErrNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	ttl := a.FreshnessTTL
	if ttl == 0 {
		ttl = 24 * time.Hour
	}
	fresh := time.Since(r.RecordedAt) < ttl
	return r.Value, fresh, nil
}

// LatestRecovery returns the freshest recovery-shaped reading for the user
// (KindRecovery or KindReadiness), or domain.ErrNotFound if neither exists.
// "Recovery" and "Readiness" are treated as equivalent here so a Whoop user
// and an Oura user both flow through the same plan-bias path.
func (s *Service) LatestRecovery(ctx context.Context, userID string) (domain.Reading, error) {
	rec, recErr := s.readings.LatestByUserAndKind(ctx, userID, domain.KindRecovery)
	rdy, rdyErr := s.readings.LatestByUserAndKind(ctx, userID, domain.KindReadiness)
	switch {
	case recErr == nil && rdyErr == nil:
		if rec.RecordedAt.After(rdy.RecordedAt) {
			return rec, nil
		}
		return rdy, nil
	case recErr == nil:
		return rec, nil
	case rdyErr == nil:
		return rdy, nil
	default:
		return domain.Reading{}, domain.ErrNotFound
	}
}

// IngestReadings persists a batch of readings. Used by the polling daemon.
// Each reading is inserted idempotently; duplicates are silently skipped.
func (s *Service) IngestReadings(ctx context.Context, userID string, readings []domain.Reading) (int, error) {
	inserted := 0
	for _, r := range readings {
		if r.ID == "" {
			r.ID = s.newID()
		}
		if r.UserID == "" {
			r.UserID = userID
		}
		if r.IngestedAt.IsZero() {
			r.IngestedAt = s.now()
		}
		if err := s.readings.Insert(ctx, r); err != nil {
			return inserted, fmt.Errorf("insert %s: %w", r.IdempotencyKey, err)
		}
		inserted++
	}
	return inserted, nil
}

// gcStatesLocked drops state entries older than 10 minutes. Caller holds
// stateMu. Cheap to call on each Connect — bounded by the rate of new
// connect attempts.
func (s *Service) gcStatesLocked() {
	cutoff := s.now().Add(-10 * time.Minute)
	for k, v := range s.states {
		if v.createdAt.Before(cutoff) {
			delete(s.states, k)
		}
	}
}

// randomID returns a 16-byte random hex string. Used as both the OAuth
// state token and (when no injected IDGenerator) the reading ID.
func randomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("biometrics: rand.Read failed: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}

// trimProvider normalizes a provider path segment.
func trimProvider(name string) string { return strings.ToLower(strings.TrimSpace(name)) }
