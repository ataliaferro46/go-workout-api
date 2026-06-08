package biometrics

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
)

// ErrMasterKeyTooShort is returned by NewTokenStore when the master key has
// fewer than 32 bytes of entropy.
var ErrMasterKeyTooShort = errors.New("biometrics master key must be at least 32 bytes")

// ErrTokenNotFound is returned by TokenStore.Load when no token exists for
// the user/provider pair.
var ErrTokenNotFound = errors.New("oauth token not found")

// TokenStore persists per-user, per-provider OAuth tokens with at-rest
// encryption. The plaintext access/refresh tokens are sealed with AES-GCM
// before they hit the underlying repository; the per-purpose AES key is
// derived from BIOMETRICS_MASTER_KEY via HKDF-SHA256.
//
// Two safety properties matter:
//
//  1. **Per-row nonce.** GCM is catastrophic under nonce reuse with the same
//     key. We generate a fresh random nonce per row and store it alongside
//     the ciphertext.
//
//  2. **Field-name AAD.** Both the access and refresh tokens share a single
//     nonce per row. The Additional Authenticated Data binds each
//     ciphertext to its field name, so an attacker cannot swap the
//     access-token bytes into the refresh-token slot without detection.
type TokenStore struct {
	repo TokenRepository
	aead cipher.AEAD
}

// TokenRepository is the storage seam TokenStore depends on. Two
// implementations: PostgresTokenRepository (production) and
// InMemoryTokenRepository (tests + in-memory mode).
//
// The interface is exported so main.go can wire either implementation,
// but the rows it carries are an unexported value type — callers cannot
// construct them by hand, only via the encrypted Save path on TokenStore.
type TokenRepository interface {
	Upsert(ctx context.Context, row TokenRow) error
	Get(ctx context.Context, userID, provider string) (TokenRow, error)
	Delete(ctx context.Context, userID, provider string) error
}

// TokenRow is the at-rest shape of an OAuth token. Plaintext tokens never
// flow through this struct.
type TokenRow struct {
	UserID, Provider                       string
	AccessTokenEnc, RefreshTokenEnc, Nonce []byte
	ExpiresAt                              time.Time
	Scopes                                 []string
	CreatedAt, UpdatedAt                   time.Time
}

// NewTokenStore returns a TokenStore using the given repository and a key
// derived from masterKey via HKDF. masterKey MUST contain ≥32 bytes of
// entropy; passing a shorter value returns ErrMasterKeyTooShort so a
// misconfigured deploy crash-loops at boot instead of running with weak
// crypto.
func NewTokenStore(repo TokenRepository, masterKey []byte) (*TokenStore, error) {
	if len(masterKey) < 32 {
		return nil, ErrMasterKeyTooShort
	}
	key, err := hkdf.Key(sha256.New, masterKey, nil, "biometrics-token-v1", 32)
	if err != nil {
		return nil, fmt.Errorf("hkdf: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}
	return &TokenStore{repo: repo, aead: aead}, nil
}

// Save encrypts and persists the token. An existing row for
// (userID, provider) is upserted — connecting Whoop a second time replaces
// the previous token without leaking the old ciphertext.
func (s *TokenStore) Save(ctx context.Context, userID, provider string, tok domain.Token) error {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("nonce: %w", err)
	}
	accessAAD := []byte(provider + ":access_token")
	refreshAAD := []byte(provider + ":refresh_token")

	row := TokenRow{
		UserID:          userID,
		Provider:        provider,
		AccessTokenEnc:  s.aead.Seal(nil, nonce, []byte(tok.AccessToken), accessAAD),
		RefreshTokenEnc: s.aead.Seal(nil, nonce, []byte(tok.RefreshToken), refreshAAD),
		Nonce:           nonce,
		ExpiresAt:       tok.ExpiresAt,
		Scopes:          tok.Scopes,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	return s.repo.Upsert(ctx, row)
}

// Load decrypts the token. A wrong master key, tampered ciphertext, or AAD
// mismatch all surface as decryption errors — the caller treats any error
// from Load as "this user must re-connect."
func (s *TokenStore) Load(ctx context.Context, userID, provider string) (domain.Token, error) {
	row, err := s.repo.Get(ctx, userID, provider)
	if err != nil {
		return domain.Token{}, err
	}
	accessAAD := []byte(provider + ":access_token")
	refreshAAD := []byte(provider + ":refresh_token")

	access, err := s.aead.Open(nil, row.Nonce, row.AccessTokenEnc, accessAAD)
	if err != nil {
		return domain.Token{}, fmt.Errorf("decrypt access: %w", err)
	}
	refresh, err := s.aead.Open(nil, row.Nonce, row.RefreshTokenEnc, refreshAAD)
	if err != nil {
		return domain.Token{}, fmt.Errorf("decrypt refresh: %w", err)
	}
	return domain.Token{
		AccessToken:  string(access),
		RefreshToken: string(refresh),
		ExpiresAt:    row.ExpiresAt,
		Scopes:       row.Scopes,
	}, nil
}

// Delete removes the token. Used when a user disconnects a provider.
func (s *TokenStore) Delete(ctx context.Context, userID, provider string) error {
	return s.repo.Delete(ctx, userID, provider)
}

// --- in-memory token repository -------------------------------------------

// InMemoryTokenRepository implements TokenRepository for the in-memory and
// test paths. Encryption is still applied (the TokenStore is the same); only
// the storage substrate changes.
type InMemoryTokenRepository struct {
	mu   sync.RWMutex
	rows map[string]TokenRow // key: userID + "\x00" + provider
}

func NewInMemoryTokenRepository() *InMemoryTokenRepository {
	return &InMemoryTokenRepository{rows: map[string]TokenRow{}}
}

func tokenKey(userID, provider string) string { return userID + "\x00" + provider }

func (r *InMemoryTokenRepository) Upsert(_ context.Context, row TokenRow) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows[tokenKey(row.UserID, row.Provider)] = row
	return nil
}

func (r *InMemoryTokenRepository) Get(_ context.Context, userID, provider string) (TokenRow, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	row, ok := r.rows[tokenKey(userID, provider)]
	if !ok {
		return TokenRow{}, ErrTokenNotFound
	}
	return row, nil
}

func (r *InMemoryTokenRepository) Delete(_ context.Context, userID, provider string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := tokenKey(userID, provider)
	if _, ok := r.rows[key]; !ok {
		return ErrTokenNotFound
	}
	delete(r.rows, key)
	return nil
}

// ListPairs implements pairLister so the polling daemon can enumerate
// connected (user, provider) tuples in in-memory mode too.
func (r *InMemoryTokenRepository) ListPairs(_ context.Context) ([]TokenPair, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]TokenPair, 0, len(r.rows))
	for _, row := range r.rows {
		out = append(out, TokenPair{UserID: row.UserID, Provider: row.Provider})
	}
	return out, nil
}
