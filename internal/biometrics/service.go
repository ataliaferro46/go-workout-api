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

// IntensitySummary captures what we can say about a workout's intensity
// from Oura HR data. Zero values mean "no data" — the front-end shows
// "not yet computed" instead of pretending.
type IntensitySummary struct {
	AvgBPM           int           `json:"avg_bpm"`
	MaxBPM           int           `json:"max_bpm"`
	SamplesCount     int           `json:"samples_count"`
	DurationMinutes  int           `json:"duration_minutes"`
	MinutesAbove140  int           `json:"minutes_above_140"`
	Source           string        `json:"source"`
	ComputedAt       time.Time     `json:"computed_at"`
}

// IntensityForWindow fetches HR samples from the user's connected
// provider (Oura today) between start and end, returning summary stats.
// Returns an empty summary (and no error) if the user has no provider
// connected — intensity is opt-in, not a hard requirement.
func (s *Service) IntensityForWindow(ctx context.Context, userID string, start, end time.Time) (IntensitySummary, error) {
	if !end.After(start) {
		return IntensitySummary{}, &domain.ValidationError{Message: "end must be after start"}
	}
	// Try Oura first; Whoop and others can be added similarly.
	tok, err := s.tokens.Load(ctx, userID, "oura")
	if err != nil {
		// No connected provider that supports HR — return empty so the
		// front-end can display "connect Oura to see intensity".
		return IntensitySummary{}, nil
	}
	p, err := s.registry.Lookup("oura")
	if err != nil {
		return IntensitySummary{}, nil
	}
	oura, ok := p.(*OuraProvider)
	if !ok {
		return IntensitySummary{}, nil
	}
	samples, err := oura.HeartRateInWindow(ctx, tok.AccessToken, start, end)
	if err != nil {
		return IntensitySummary{}, err
	}
	out := computeIntensity(samples, start, end)
	out.Source = "oura"
	out.ComputedAt = s.now()
	return out, nil
}

// computeIntensity reduces a list of HR samples to a single intensity
// summary. Time-above-140 is calculated by assuming each sample
// represents the interval until the next sample (~5 min by default).
func computeIntensity(samples []HRSample, start, end time.Time) IntensitySummary {
	out := IntensitySummary{
		SamplesCount:    len(samples),
		DurationMinutes: int(end.Sub(start).Minutes()),
	}
	if len(samples) == 0 {
		return out
	}
	sumBPM := 0
	maxBPM := 0
	above140Seconds := 0.0
	for i, s := range samples {
		sumBPM += s.BPM
		if s.BPM > maxBPM {
			maxBPM = s.BPM
		}
		// Estimate the interval this sample represents.
		var intervalSec float64
		if i+1 < len(samples) {
			intervalSec = samples[i+1].RecordedAt.Sub(s.RecordedAt).Seconds()
		} else {
			intervalSec = end.Sub(s.RecordedAt).Seconds()
		}
		if intervalSec < 0 {
			intervalSec = 0
		}
		if intervalSec > 600 {
			intervalSec = 600 // cap any gap at 10 min so we don't credit huge holes
		}
		if s.BPM >= 140 {
			above140Seconds += intervalSec
		}
	}
	out.AvgBPM = sumBPM / len(samples)
	out.MaxBPM = maxBPM
	out.MinutesAbove140 = int(above140Seconds / 60.0)
	return out
}

// SyncNow forces an immediate sync for a single (user, provider) pair,
// bypassing the polling daemon's 15-minute interval. Returns the number of
// readings ingested. This is what the Settings page's "Sync now" button
// calls — useful as a UX affordance and as a diagnostic when a connection
// looks linked but readings are missing.
func (s *Service) SyncNow(ctx context.Context, userID, providerName string) (int, error) {
	p, err := s.registry.Lookup(providerName)
	if err != nil {
		return 0, err
	}
	tok, err := s.tokens.Load(ctx, userID, providerName)
	if err != nil {
		return 0, err
	}
	state, err := s.syncState.Get(ctx, userID, providerName)
	since := s.now().Add(-7 * 24 * time.Hour) // fall back to last 7 days
	if err == nil {
		since = state.LastSyncedAt
	} else if !errors.Is(err, ErrSyncStateNotFound) {
		return 0, err
	}
	readings, err := p.LatestSince(ctx, tok.AccessToken, since)
	if err != nil {
		return 0, err
	}
	n, err := s.IngestReadings(ctx, userID, readings)
	if err != nil {
		return 0, err
	}
	if err := s.syncState.Set(ctx, SyncState{
		UserID:       userID,
		Provider:     providerName,
		LastSyncedAt: s.now(),
	}); err != nil {
		return n, err
	}
	return n, nil
}

// ProviderStatus is the per-provider view the front-end uses to render the
// Settings page (Connect / Disconnect buttons, last-sync timestamp).
type ProviderStatus struct {
	Name         string     `json:"name"`
	Connected    bool       `json:"connected"`
	LastSyncedAt *time.Time `json:"last_synced_at,omitempty"`
}

// ListProviders returns one ProviderStatus per registered provider, with
// per-user connection state filled in. "mock" is filtered out — it's a dev/
// test fixture, not something to expose in the user-facing UI.
func (s *Service) ListProviders(ctx context.Context, userID string) ([]ProviderStatus, error) {
	names := s.registry.Names()
	out := make([]ProviderStatus, 0, len(names))
	for _, name := range names {
		if name == "mock" {
			continue
		}
		ps := ProviderStatus{Name: name}
		if _, err := s.tokens.Load(ctx, userID, name); err == nil {
			ps.Connected = true
			if state, err := s.syncState.Get(ctx, userID, name); err == nil {
				t := state.LastSyncedAt
				ps.LastSyncedAt = &t
			}
		}
		out = append(out, ps)
	}
	return out, nil
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
