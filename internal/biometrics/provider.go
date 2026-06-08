// Package biometrics integrates with third-party wearables (Whoop, Oura) to
// ingest recovery, sleep, and strain metrics. The shape mirrors the rest of
// the codebase: a Provider interface with concrete implementations behind it,
// a Repository for persistence, a Service that ties them together, and an
// HTTP handler for the user-facing surface (OAuth connect, webhook receive,
// latest-reading lookup).
//
// See docs/specs/01-biometrics.md for the architectural overview and
// ARCHITECTURE.md ADR-047..ADR-052 for the decision records.
package biometrics

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
)

// ErrUnknownProvider is returned by the Registry when a name doesn't match a
// registered provider — typically because the route handler received a path
// segment that doesn't correspond to a configured integration.
var ErrUnknownProvider = errors.New("unknown provider")

// ErrInvalidSignature is returned by webhook verification when the HMAC does
// not match. The handler maps it to 401.
var ErrInvalidSignature = errors.New("invalid webhook signature")

// Provider abstracts a third-party biometric integration. The interface is
// deliberately narrow: OAuth handshake, refresh, polling, webhook verify.
// Provider implementations hold their own HTTP client and credentials.
type Provider interface {
	// Name returns the URL-safe provider identifier (e.g., "whoop", "oura").
	// It must be stable across releases — it appears in the
	// oauth_tokens.provider column and in routes like /v1/biometrics/connect/{provider}.
	Name() string

	// AuthURL returns the URL the user should be redirected to in order to
	// authorize this provider. state is a CSRF-defense token that the
	// callback handler will verify on the return trip.
	AuthURL(state string) string

	// ExchangeCode trades the authorization code returned to the OAuth
	// callback for an access/refresh token pair.
	ExchangeCode(ctx context.Context, code string) (domain.Token, error)

	// Refresh trades a refresh token for a fresh access token (and possibly
	// a new refresh token). Implementations should rotate the refresh token
	// when the provider returns one.
	Refresh(ctx context.Context, refreshToken string) (domain.Token, error)

	// LatestSince fetches readings recorded after `since`. The access token
	// is provided plaintext; the caller (Service) is responsible for
	// loading and decrypting it.
	LatestSince(ctx context.Context, accessToken string, since time.Time) ([]domain.Reading, error)

	// VerifyWebhook checks the provider's signature on an incoming webhook
	// body. Returns ErrInvalidSignature for any verification failure so
	// downstream handlers can map uniformly.
	VerifyWebhook(headers http.Header, body []byte) error
}

// Registry maps provider names to Provider implementations. main.go builds
// it at boot from environment configuration; the Service consults it on
// every OAuth/refresh/sync call.
type Registry map[string]Provider

// NewRegistry returns an empty registry. Use Register to add providers.
func NewRegistry() Registry { return make(Registry) }

// Register adds a provider keyed by its Name(). A second Register with the
// same name overwrites the first (no-op in practice since main.go calls
// each provider's Register at most once).
func (r Registry) Register(p Provider) { r[p.Name()] = p }

// Lookup returns the provider registered under name or ErrUnknownProvider.
func (r Registry) Lookup(name string) (Provider, error) {
	p, ok := r[name]
	if !ok {
		return nil, ErrUnknownProvider
	}
	return p, nil
}

// Names returns the registered provider names. Stable order is not
// guaranteed; callers should sort if they care.
func (r Registry) Names() []string {
	out := make([]string, 0, len(r))
	for name := range r {
		out = append(out, name)
	}
	return out
}
