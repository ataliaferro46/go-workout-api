package biometrics

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
)

// MockProvider is a Provider that returns canned data and a deterministic
// signature scheme. It exists for two reasons:
//
//  1. Tests can exercise the full OAuth + sync + webhook plumbing without
//     external network calls.
//  2. A developer running `go run` against the in-memory mode gets a
//     working /v1/biometrics/* surface that returns plausible recovery
//     values, so recovery-aware plan generation can be demonstrated end-to-
//     end without any third-party credentials.
//
// All MockProvider state is safe to mutate from tests via the public
// methods; the mutex protects against concurrent test setup.
type MockProvider struct {
	mu sync.Mutex
	// SignatureKey is the shared secret used for VerifyWebhook. Tests sign
	// their fixture bodies with this same key.
	SignatureKey string
	// nextReadings is the list of readings LatestSince will return next.
	// Tests prime this before invoking the polling daemon.
	nextReadings []domain.Reading
}

// NewMockProvider returns a MockProvider with the given HMAC signing key.
func NewMockProvider(signatureKey string) *MockProvider {
	return &MockProvider{SignatureKey: signatureKey}
}

// Name returns "mock".
func (m *MockProvider) Name() string { return "mock" }

// AuthURL returns a fake auth URL embedding the state for round-trip testing.
func (m *MockProvider) AuthURL(state string) string {
	return "https://mock.example.com/oauth?state=" + state
}

// ExchangeCode returns a deterministic token derived from the code so tests
// can assert tokens were exchanged for the right code.
func (m *MockProvider) ExchangeCode(_ context.Context, code string) (domain.Token, error) {
	return domain.Token{
		AccessToken:  "mock-access-" + code,
		RefreshToken: "mock-refresh-" + code,
		ExpiresAt:    time.Now().Add(time.Hour),
		Scopes:       []string{"read:recovery", "read:sleep"},
	}, nil
}

// Refresh returns a fresh deterministic token from the refresh token.
func (m *MockProvider) Refresh(_ context.Context, refreshToken string) (domain.Token, error) {
	return domain.Token{
		AccessToken:  "mock-access-refreshed-" + refreshToken,
		RefreshToken: refreshToken, // mocks do not rotate
		ExpiresAt:    time.Now().Add(time.Hour),
		Scopes:       []string{"read:recovery", "read:sleep"},
	}, nil
}

// SetNextReadings primes the next LatestSince call. Tests use this to inject
// fixtures.
func (m *MockProvider) SetNextReadings(readings []domain.Reading) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextReadings = readings
}

// LatestSince returns the readings previously primed via SetNextReadings.
// `since` is ignored — tests are responsible for filtering before
// SetNextReadings if they want to simulate watermark behavior.
func (m *MockProvider) LatestSince(_ context.Context, _ string, _ time.Time) ([]domain.Reading, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := m.nextReadings
	m.nextReadings = nil
	return out, nil
}

// VerifyWebhook implements HMAC-SHA256 verification against SignatureKey.
// Tests sign their bodies with HMACSign below.
//
// Expected header: X-Mock-Signature: hex(HMAC-SHA256(SignatureKey, body))
func (m *MockProvider) VerifyWebhook(headers http.Header, body []byte) error {
	got := headers.Get("X-Mock-Signature")
	if got == "" || !hmac.Equal([]byte(got), []byte(HMACSign(m.SignatureKey, body))) {
		return ErrInvalidSignature
	}
	return nil
}

// HMACSign computes the hex-encoded HMAC-SHA256 of body under key. Exposed
// so tests can construct valid signatures and so the WhoopProvider /
// OuraProvider can share the implementation when their signature schemes
// are also HMAC-SHA256 (which both are, with provider-specific header
// names).
func HMACSign(key string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// MakeReading is a convenience constructor that tests use to build a
// Reading with sensible defaults filled in.
func MakeReading(userID string, kind domain.Kind, value float64, recordedAt time.Time, idem string) domain.Reading {
	raw, _ := json.Marshal(map[string]any{
		"kind": kind, "value": value, "recorded_at": recordedAt,
	})
	return domain.Reading{
		ID:             idem, // ID = idempotency key in the mock; production uses random UUIDs
		UserID:         userID,
		Provider:       "mock",
		Kind:           kind,
		Value:          value,
		RecordedAt:     recordedAt,
		IngestedAt:     time.Now().UTC(),
		IdempotencyKey: idem,
		RawPayload:     raw,
	}
}
