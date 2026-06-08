package biometrics

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
)

func validMasterKey() []byte {
	// 32 bytes — meets the minimum.
	return []byte("0123456789abcdef0123456789abcdef")
}

func TestTokenStore_RejectsShortMasterKey(t *testing.T) {
	_, err := NewTokenStore(NewInMemoryTokenRepository(), []byte("short"))
	if err != ErrMasterKeyTooShort {
		t.Fatalf("expected ErrMasterKeyTooShort, got %v", err)
	}
}

func TestTokenStore_RoundTripsBothTokens(t *testing.T) {
	store := mustNewTokenStore(t)
	want := domain.Token{
		AccessToken:  "access-secret-12345",
		RefreshToken: "refresh-secret-67890",
		ExpiresAt:    time.Now().Add(time.Hour).Round(time.Second),
		Scopes:       []string{"read:recovery", "read:sleep"},
	}
	if err := store.Save(context.Background(), "u1", "whoop", want); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := store.Load(context.Background(), "u1", "whoop")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.AccessToken != want.AccessToken {
		t.Errorf("AccessToken = %q, want %q", got.AccessToken, want.AccessToken)
	}
	if got.RefreshToken != want.RefreshToken {
		t.Errorf("RefreshToken = %q, want %q", got.RefreshToken, want.RefreshToken)
	}
}

func TestTokenStore_DifferentMasterKeyCannotDecrypt(t *testing.T) {
	saveStore := mustNewTokenStore(t)
	if err := saveStore.Save(context.Background(), "u1", "whoop", domain.Token{
		AccessToken: "secret", RefreshToken: "secret-refresh",
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Load through a different store sharing the underlying repo but using
	// a different master key. Decryption MUST fail — that's the point of
	// at-rest encryption.
	otherKey := append([]byte{}, validMasterKey()...)
	otherKey[0] ^= 0xff
	loadStore, err := NewTokenStore(saveStore.repo, otherKey)
	if err != nil {
		t.Fatalf("loadStore: %v", err)
	}
	if _, err := loadStore.Load(context.Background(), "u1", "whoop"); err == nil {
		t.Fatal("expected decryption to fail with wrong master key")
	}
}

func TestTokenStore_AADPreventsCrossFieldCopy(t *testing.T) {
	// Setup: save a token, then maliciously copy access ciphertext into
	// the refresh slot. The Load path's AAD-on-decrypt must reject it.
	store := mustNewTokenStore(t)
	_ = store.Save(context.Background(), "u1", "whoop", domain.Token{
		AccessToken: "access", RefreshToken: "refresh",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	repo, ok := store.repo.(*InMemoryTokenRepository)
	if !ok {
		t.Fatal("expected InMemoryTokenRepository")
	}
	repo.mu.Lock()
	row := repo.rows[tokenKey("u1", "whoop")]
	row.RefreshTokenEnc = row.AccessTokenEnc // swap in access bytes
	repo.rows[tokenKey("u1", "whoop")] = row
	repo.mu.Unlock()

	if _, err := store.Load(context.Background(), "u1", "whoop"); err == nil {
		t.Fatal("expected AAD-on-decrypt to reject cross-field swap")
	} else if !strings.Contains(err.Error(), "decrypt") {
		t.Errorf("expected decrypt-error, got %v", err)
	}
}

func TestTokenStore_DeleteRemovesRow(t *testing.T) {
	store := mustNewTokenStore(t)
	_ = store.Save(context.Background(), "u1", "whoop", domain.Token{
		AccessToken: "a", RefreshToken: "b",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	if err := store.Delete(context.Background(), "u1", "whoop"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := store.Load(context.Background(), "u1", "whoop"); err != ErrTokenNotFound {
		t.Fatalf("expected ErrTokenNotFound, got %v", err)
	}
}

func mustNewTokenStore(t *testing.T) *TokenStore {
	t.Helper()
	store, err := NewTokenStore(NewInMemoryTokenRepository(), validMasterKey())
	if err != nil {
		t.Fatalf("NewTokenStore: %v", err)
	}
	return store
}
